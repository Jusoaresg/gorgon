package web

import (
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jusoaresg/gorgon/config"
	prowlarrSchema "github.com/jusoaresg/gorgon/external/prowlarr/schema"
	prowlarrService "github.com/jusoaresg/gorgon/external/prowlarr/service"
	qbittorrentService "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	episodeEvents "github.com/jusoaresg/gorgon/internal/episode/events"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	episodeService "github.com/jusoaresg/gorgon/internal/episode/service"
	episodeTorrentModel "github.com/jusoaresg/gorgon/internal/episode_torrent/model"
	"github.com/jusoaresg/gorgon/internal/filter"
	filterService "github.com/jusoaresg/gorgon/internal/filter/service"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/jusoaresg/gorgon/utils"
	"github.com/labstack/echo/v4"
)

func (h *Handler) ChangeEpisodeTrackingModal(c echo.Context) error {
	epIdStr := c.Param("id")
	epIdInt, err := strconv.Atoi(epIdStr)
	if err != nil {
		return err
	}

	episode, err := h.EpisodeRepo.GetByID(int64(epIdInt))
	if err != nil {
		return err
	}

	return c.Render(http.StatusOK, "episode-tracking-modal", episode)
}

type interactiveSearchData struct {
	EpisodeID   int64
	EpisodeName string
	Season      int
	Number      int
	AutoSearch  bool
}

func (h *Handler) InteractiveSearchModal(c echo.Context) error {
	epIdStr := c.Param("id")
	epId, err := strconv.ParseInt(epIdStr, 10, 64)
	if err != nil {
		return err
	}

	episode, err := h.EpisodeRepo.GetByID(epId)
	if err != nil {
		return err
	}

	return c.Render(http.StatusOK, "interactive-search-modal", interactiveSearchData{
		EpisodeID:   episode.ID,
		EpisodeName: episode.Name,
		Season:      episode.Season,
		Number:      episode.Number,
		AutoSearch:  true,
	})
}

type aliasProgressItem struct {
	Name    string
	Encoded string
	Delay   string // " delay:3s" for low-priority aliases, empty for high-priority
}

var latinAliasRegex = regexp.MustCompile(`^[a-zA-Z0-9\s\-_!?',.:]+$`)

type searchProgressData struct {
	EpisodeID int64
	Season    int
	Number    int
	Titles    []aliasProgressItem
}

func (h *Handler) SearchEpisodeResults(c echo.Context) error {
	epIdStr := c.Param("id")
	epId, err := strconv.ParseInt(epIdStr, 10, 64)
	if err != nil {
		return err
	}

	logger := config.GetLogger()

	episode, err := h.EpisodeRepo.GetByID(epId)
	if err != nil {
		return err
	}

	show, err := h.ShowRepo.GetById(episode.ShowID)
	if err != nil {
		return err
	}

	aliases, err := h.ShowAliasesRepo.ListByShowID(show.ID)
	if err != nil {
		logger.Error("error fetching show aliases", slog.String("error", err.Error()))
		return err
	}

	titles := []aliasProgressItem{
		{Name: utils.NormalizeTitle(show.Name), Encoded: url.QueryEscape(utils.NormalizeTitle(show.Name))},
	}

	for _, alias := range aliases {
		if !latinAliasRegex.MatchString(alias.Alias) {
			continue
		}
		titles = append(titles, aliasProgressItem{
			Name:    alias.Alias,
			Encoded: url.QueryEscape(alias.Alias),
		})
	}

	for _, alias := range aliases {
		if latinAliasRegex.MatchString(alias.Alias) {
			continue
		}
		titles = append(titles, aliasProgressItem{
			Name:    alias.Alias,
			Encoded: url.QueryEscape(alias.Alias),
			Delay:   " delay:3s",
		})
	}

	return c.Render(http.StatusOK, "search-results-progress", searchProgressData{
		EpisodeID: epId,
		Season:    episode.Season,
		Number:    episode.Number,
		Titles:    titles,
	})
}

type EvaluatedResult struct {
	prowlarrSchema.SearchResponse
	EpisodeMatched  bool
	PassedFilter    bool
	Score           int
	RejectionReason string
	Metadata        filter.ReleaseMetadata
}

type searchAliasResultsData struct {
	Alias      string
	Results    []EvaluatedResult
	EpisodeID  int64
	TopScore   int
	MatchCount int
}

func (h *Handler) SearchAliasResult(c echo.Context) error {
	epIdStr := c.Param("id")
	epId, err := strconv.ParseInt(epIdStr, 10, 64)
	if err != nil {
		return err
	}

	title := c.QueryParam("t")
	if title == "" {
		return c.NoContent(http.StatusBadRequest)
	}

	seasonStr := c.QueryParam("s")
	numberStr := c.QueryParam("n")
	season, _ := strconv.Atoi(seasonStr)
	number, _ := strconv.Atoi(numberStr)

	logger := config.GetLogger()

	episode, err := h.EpisodeRepo.GetByID(epId)
	if err != nil {
		logger.Error("error fetching episode for search", slog.String("error", err.Error()))
		return c.Render(http.StatusOK, "search-alias-results", searchAliasResultsData{
			Alias:      title,
			Results:    nil,
			EpisodeID:  epId,
			TopScore:   0,
			MatchCount: 0,
		})
	}

	show, err := h.ShowRepo.GetById(episode.ShowID)
	if err != nil {
		logger.Error("error fetching show for search", slog.String("error", err.Error()))
		return c.Render(http.StatusOK, "search-alias-results", searchAliasResultsData{
			Alias:      title,
			Results:    nil,
			EpisodeID:  epId,
			TopScore:   0,
			MatchCount: 0,
		})
	}

	settings, _ := filterService.ResolveSettings(h.DB, show.ID)
	profile, _ := filterService.ResolveProfile(h.DB, settings)
	ctx, _ := filterService.BuildContext(h.DB, show, season, number, settings)

	prowlarrIndexerService := prowlarrService.NewProwlarrIndexerService(logger)
	var indexers []prowlarrSchema.IndexerResponse
	if err := prowlarrIndexerService.GetIndexers(&indexers); err != nil {
		logger.Error("error fetching prowlarr indexers", slog.String("error", err.Error()))
		return c.Render(http.StatusOK, "search-alias-results", searchAliasResultsData{
			Alias:      title,
			Results:    nil,
			EpisodeID:  epId,
			TopScore:   0,
			MatchCount: 0,
		})
	}

	var indexerIds []int
	for _, indexer := range indexers {
		if indexer.Enabled {
			indexerIds = append(indexerIds, indexer.Id)
		}
	}

	searchService, err := prowlarrService.NewInteractiveProwlarrSearchService(logger)
	if err != nil {
		logger.Error("error initializing prowlarr service", slog.String("error", err.Error()))
		return c.Render(http.StatusOK, "search-alias-results", searchAliasResultsData{
			Alias:      title,
			Results:    nil,
			EpisodeID:  epId,
			TopScore:   0,
			MatchCount: 0,
		})
	}

	patterns := filterService.SearchPatterns(profile, settings.ShowType)

	var queries []string
	for _, pattern := range patterns {
		q, err := filter.ExpandQuery(pattern, ctx, title)
		if err == nil && strings.TrimSpace(q) != "" {
			queries = append(queries, q)
		}
	}
	if len(queries) == 0 {
		queries = []string{title}
	}

	seen := make(map[string]struct{})
	var rawResults []prowlarrSchema.SearchResponse

	for _, query := range queries {
		searchKey := prowlarrSchema.SearchByTypeRequest{
			Query: query,
			Type:  "search",
		}

		var res []prowlarrSchema.SearchResponse
		if err := searchService.SearchByType(&searchKey, &res, indexerIds...); err != nil {
			logger.Error("error searching prowlarr",
				slog.String("query", query),
				slog.String("error", err.Error()),
			)
			continue
		}

		for _, r := range res {
			idKey := r.InfoHash
			if idKey == "" {
				idKey = r.Guid
			}
			if idKey == "" {
				idKey = r.Filename
			}
			if _, exists := seen[idKey]; !exists {
				seen[idKey] = struct{}{}
				rawResults = append(rawResults, r)
			}
		}
	}

	topScore := 0
	matchCount := 0
	evaluated := make([]EvaluatedResult, 0, len(rawResults))
	for _, r := range rawResults {
		epMatch := filter.MatchEpisode(r.Filename, season, number, settings.ShowType)
		eval := filter.Evaluate(profile, ctx, r.Filename)
		score := episodeService.BaseScore(r) + eval.PreferredScore

		meta := filter.ParseReleaseMetadata(r.Filename)
		item := EvaluatedResult{
			SearchResponse: r,
			EpisodeMatched: epMatch,
			Score:          score,
			Metadata:       meta,
		}

		if !epMatch {
			item.PassedFilter = false
			item.RejectionReason = "Episode mismatch"
		} else if !eval.Passed {
			item.PassedFilter = false
			item.RejectionReason = eval.RejectedReason
		} else {
			item.PassedFilter = true
			matchCount++
			if score > topScore {
				topScore = score
			}
		}

		evaluated = append(evaluated, item)
	}

	sort.Slice(evaluated, func(i, j int) bool {
		if evaluated[i].PassedFilter != evaluated[j].PassedFilter {
			return evaluated[i].PassedFilter
		}
		if evaluated[i].EpisodeMatched != evaluated[j].EpisodeMatched {
			return evaluated[i].EpisodeMatched
		}
		return evaluated[i].Score > evaluated[j].Score
	})

	return c.Render(http.StatusOK, "search-alias-results", searchAliasResultsData{
		Alias:      title,
		Results:    evaluated,
		EpisodeID:  epId,
		TopScore:   topScore,
		MatchCount: matchCount,
	})
}

func (h *Handler) DownloadEpisodeTorrent(c echo.Context) error {
	epIdStr := c.Param("id")
	epId, err := strconv.ParseInt(epIdStr, 10, 64)
	if err != nil {
		schemas.SendError(c, 400, "Invalid episode ID")
		return nil
	}

	var request struct {
		Guid        string `json:"guid"`
		InfoHash    string `json:"infoHash"`
		Title       string `json:"title,omitempty"`
		Indexer     string `json:"indexer,omitempty"`
		InfoUrl     string `json:"infoUrl,omitempty"`
		PublishDate string `json:"publishDate,omitempty"`
	}
	if err := c.Bind(&request); err != nil {
		schemas.SendError(c, 400, "Invalid request")
		return nil
	}

	logger := config.GetLogger()

	ep, err := h.EpisodeRepo.GetByID(epId)
	if err != nil {
		logger.Error("error fetching episode", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Episode not found")
		return nil
	}

	torrentService, err := qbittorrentService.NewQBittorrentService(logger)
	if err != nil {
		logger.Error("error initializing qbittorrent service", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Torrent client not available")
		return nil
	}

	if err := torrentService.AddTorrent(request.Guid); err != nil {
		logger.Error("error adding torrent", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Failed to add torrent")
		return nil
	}

	ep.Tracking = episodeModel.TrackingSnatched

	episodeTorrent := episodeTorrentModel.EpisodeTorrent{
		EpisodeId:   ep.ID,
		Hash:        request.InfoHash,
		Title:       request.Title,
		Indexer:     request.Indexer,
		InfoUrl:     request.InfoUrl,
		PublishDate: request.PublishDate,
		CreatedAt:   time.Now().Unix(),
	}
	if _, err := h.EpisodeTorrentRepo.Upsert(episodeTorrent); err != nil {
		logger.Error("error saving episode torrent", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Failed to save episode torrent")
		return nil
	}

	if err := h.EpisodeRepo.Update(ep); err != nil {
		logger.Error("error updating episode tracking", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Failed to update episode")
		return nil
	}

	episodeEvents.EmitEpisodeTrackingUpdatedEvent(ep.ID, ep.Tracking, episodeTorrent.InfoUrl)

	logger.Info("torrent added and episode snatched",
		slog.Int64("episode_id", ep.ID),
		slog.String("hash", request.InfoHash),
	)

	schemas.SendSuccess(c, "Download started", nil)
	return nil
}
