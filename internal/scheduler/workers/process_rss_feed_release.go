package workers

import (
	"log/slog"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	episodeService "github.com/jusoaresg/gorgon/internal/episode/service"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	filter "github.com/jusoaresg/gorgon/internal/filter"
	filterService "github.com/jusoaresg/gorgon/internal/filter/service"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/utils"
)

type RssReleaseProcessor struct {
	db                 *sqlx.DB
	showRepo           showRepository.ShowRepositoryInterface
	seasonRepo         seasonRepository.SeasonRepositoryInterface
	episodeTorrentRepo episodeTorrentRepository.EpisodeTorrentRepositoryInterface
	downloader         episodeService.EpisodeDownloaderInterface
}

func NewRssReleaseProcessor(db *sqlx.DB) RssReleaseProcessor {
	return RssReleaseProcessor{
		db:                 db,
		showRepo:           showRepository.NewShowRepository(db),
		seasonRepo:         seasonRepository.NewSeasonRepository(db),
		episodeTorrentRepo: episodeTorrentRepository.NewEpisodeTorrentRepository(db),
		downloader:         episodeService.NewEpisodeDownloader(db),
	}
}

func (rss *RssReleaseProcessor) RssProcessRelease(ep model.Episode, responses []schema.SearchResponse) error {
	logger := config.GetLogger().WithGroup("worker").With("name", "RssProcessRelease")

	show, err := rss.showRepo.GetById(ep.ShowID)
	if err != nil {
		return err
	}

	settings, err := filterService.ResolveSettings(rss.db, show.ID)
	if err != nil {
		return err
	}

	profile, err := filterService.ResolveProfile(rss.db, settings)
	if err != nil {
		return err
	}

	ctx, err := filterService.BuildContext(rss.db, show, ep.Season, ep.Number, settings)
	if err != nil {
		return err
	}

	termKeys := make([]string, 0)
	for _, pattern := range filterService.SearchPatterns(profile, settings.ShowType) {
		for _, name := range ctx.AllNames() {
			query, err := filter.ExpandQuery(pattern, ctx, name)
			if err != nil {
				continue
			}
			termKeys = append(termKeys, strings.ToLower(query))
		}
	}

	var processedResponses []schema.SearchResponse

	responseSeen := make(map[string]struct{})
	for _, response := range responses {
		title := utils.NormalizeTitle(response.Title)

		for _, term := range termKeys {
			if strings.Contains(title, term) {
				if _, seen := responseSeen[title]; !seen {
					processedResponses = append(processedResponses, response)
					responseSeen[title] = struct{}{}
				}
				break
			}
		}
	}

	processedResponses = episodeService.FilterAndScoreResponses(processedResponses, profile, ctx)
	if len(processedResponses) == 0 {
		logger.Debug("No matching releases after filtering")
		return nil
	}

	bestResponse := processedResponses[0]
	bestScore := episodeService.CalculateScore(bestResponse, profile, ctx)

	// Handle upgrades for episodes already snatched or downloaded
	if ep.Tracking == model.TrackingDownloaded || ep.Tracking == model.TrackingSnatched {
		existingTorrent, err := rss.episodeTorrentRepo.GetByEpisodeID(ep.ID)
		existingScore := 0
		if err == nil {
			existingScore = existingTorrent.Score
		}

		if !episodeService.ShouldUpgrade(ep.Tracking, profile, existingScore, bestScore) {
			logger.Debug("Candidate release does not qualify for upgrade",
				slog.Int64("episode_id", ep.ID),
				slog.Int("existing_score", existingScore),
				slog.Int("candidate_score", bestScore),
			)
			return nil
		}

		logger.Info("Triggering release upgrade via RSS feed",
			slog.Int64("episode_id", ep.ID),
			slog.Int64("show_id", ep.ShowID),
			slog.Int("old_score", existingScore),
			slog.Int("new_score", bestScore),
			slog.Int("cutoff", profile.UpgradeUntilScore),
			slog.String("new_release", bestResponse.Title),
		)

		return rss.downloader.UpgradeEpisode(ep, bestResponse, bestScore)
	}

	// Standard download for wanted or missing episodes
	logger.Info(
		"Downloading episode",
		slog.String("title", bestResponse.Title),
		slog.Int64("show_id", ep.ShowID),
		slog.Int64("episode_id", ep.ID),
		slog.Int("score", bestScore),
	)
	return rss.downloader.DownloadEpisode(ep, bestResponse, bestScore)
}
