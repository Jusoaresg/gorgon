package web

import (
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	"github.com/jusoaresg/gorgon/config"
	prowlarrSchema "github.com/jusoaresg/gorgon/external/prowlarr/schema"
	episodeService "github.com/jusoaresg/gorgon/internal/episode/service"
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

type searchAliasResultsData struct {
	Alias      string
	Results    []episodeService.EvaluatedResult
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

	evaluated, err := h.EpisodeSearchSvc.Searcher.SearchAliasForEpisode(episode, show, title, h.DB, episodeService.SearchOptions{
		SkipCooldown: true,
		StopOnGood:   false,
	})
	if err != nil {
		logger.Error("error searching alias for episode", slog.String("title", title), slog.String("error", err.Error()))
		return c.Render(http.StatusOK, "search-alias-results", searchAliasResultsData{
			Alias:      title,
			Results:    nil,
			EpisodeID:  epId,
			TopScore:   0,
			MatchCount: 0,
		})
	}

	topScore := 0
	matchCount := 0
	for _, item := range evaluated {
		if item.PassedFilter {
			matchCount++
			if item.Score > topScore {
				topScore = item.Score
			}
		}
	}

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

	searchResponse := prowlarrSchema.SearchResponse{
		Guid:        request.Guid,
		InfoHash:    request.InfoHash,
		Title:       request.Title,
		Filename:    request.Title,
		Indexer:     request.Indexer,
		InfoUrl:     request.InfoUrl,
		PublishDate: request.PublishDate,
	}

	show, _ := h.ShowRepo.GetById(ep.ShowID)
	settings, _ := filterService.ResolveSettings(h.DB, show.ID)
	profile, _ := filterService.ResolveProfile(h.DB, settings)
	ctx, _ := filterService.BuildContext(h.DB, show, ep.Season, ep.Number, settings)
	manualScore := episodeService.CalculateScore(searchResponse, profile, ctx)

	if err := h.EpisodeSearchSvc.Downloader.DownloadEpisodeTorrent(ep, searchResponse, manualScore); err != nil {
		logger.Error("failed to download episode torrent", slog.Int64("episode_id", ep.ID), slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Failed to download torrent: "+err.Error())
		return nil
	}

	schemas.SendSuccess(c, "Download started", nil)
	return nil
}

