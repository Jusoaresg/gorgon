package workers

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
	"github.com/jusoaresg/gorgon/external/prowlarr/service"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	"github.com/jusoaresg/gorgon/internal/episode/repository"
	episodeService "github.com/jusoaresg/gorgon/internal/episode/service"
)

const (
	rssFetchLimit     = 100
	rssPollInterval   = 3 * time.Minute
	rssSeenCacheTTL   = 6 * time.Hour
)

// NOTE: Responses could be a pointer
type EpisodeJob struct {
	Episode   model.Episode
	Responses []schema.SearchResponse
}

func releaseIdentifier(r schema.SearchResponse) string {
	if r.InfoHash != "" {
		return r.InfoHash
	}
	if r.Guid != "" {
		return r.Guid
	}
	return r.Title
}

func StartRssEpisodeFetcherWorker(ctx context.Context, workerCount int, prowlarrService *service.ProwlarrSearchService) {
	logger := config.GetLogger().WithGroup("worker").With("name", "StartRssFeedWorker")
	rssProcessor := NewRssReleaseProcessor(config.GetSQLite())
	searcher := &episodeService.EpisodeSearcher{}
	seenReleases := make(map[string]time.Time)

	jobChan := make(chan EpisodeJob, workerCount)

	var wg sync.WaitGroup

	logger.Info(
		"Starting rss feed worker",
		slog.Int("worker_count", workerCount),
	)

	for i := range workerCount {
		workerID := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("Worker started", slog.Int("worker_id", workerID))

			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobChan:
					if !ok {
						return
					}
					err := rssProcessor.RssProcessRelease(job.Episode, job.Responses)
					if err != nil {
						logger.Error(
							"failed to process episode",
							slog.String("error", err.Error()),
							slog.Int64("show_id", job.Episode.ShowID),
							slog.Int64("episode_id", job.Episode.ID),
						)
						continue
					}
				}
			}
		}()
	}

	ticker := time.NewTicker(rssPollInterval)
	defer func() {
		ticker.Stop()
		close(jobChan)
		wg.Wait()
		logger.Info("RSS feed worker stopped cleanly")
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logger.Info("Polling RSS feed for latest indexer releases")

			episodes := fetchRssFeedWantedEpisodes()
			if len(episodes) == 0 {
				logger.Debug("No released tracked episodes eligible for RSS check")
				continue
			}

			// Prune expired entries from seenReleases cache
			now := time.Now()
			for k, ts := range seenReleases {
				if now.Sub(ts) > rssSeenCacheTTL {
					delete(seenReleases, k)
				}
			}

			currentResponses, err := searcher.FetchRssFeed(rssFetchLimit)
			if err != nil && prowlarrService != nil {
				// Fallback to direct prowlarrService if indexer lookup fails
				err = prowlarrService.FetchRssFeed(rssFetchLimit, &currentResponses)
			}
			if err != nil {
				logger.Error("Failed to fetch RSS feed from Prowlarr", slog.String("error", err.Error()))
				continue
			}
			if len(currentResponses) == 0 {
				continue
			}

			matchedJobs := rssProcessor.MatchReleasesToEpisodes(episodes, currentResponses)
			for _, job := range matchedJobs {
				// Filter out releases already evaluated for this specific episode and tracking state
				var unseenForEpisode []schema.SearchResponse
				for _, r := range job.Responses {
					cacheKey := fmt.Sprintf("%d:%s:%s", job.Episode.ID, job.Episode.Tracking, releaseIdentifier(r))
					if _, seen := seenReleases[cacheKey]; !seen {
						seenReleases[cacheKey] = now
						unseenForEpisode = append(unseenForEpisode, r)
					}
				}
				if len(unseenForEpisode) == 0 {
					continue
				}

				job.Responses = unseenForEpisode
				logger.Info(
					"Queuing RSS matched episode for evaluation",
					slog.Int64("episode_id", job.Episode.ID),
					slog.Int64("show_id", job.Episode.ShowID),
					slog.String("name", job.Episode.Name),
					slog.Int("matched_releases", len(unseenForEpisode)),
				)
				select {
				case <-ctx.Done():
					return
				case jobChan <- job:
				}
			}
		}
	}
}

func fetchRssFeedWantedEpisodes() []model.Episode {
	logger := config.GetLogger().WithGroup("worker").With("name", "fetchWantedEps")
	db := config.GetSQLite()

	episodeRepository := repository.NewEpisodeRepository(db)

	episodes, err := episodeRepository.ListReleasedByTracking(
		model.TrackingWanted,
		model.TrackingMissing,
		model.TrackingSnatched,
		model.TrackingDownloaded,
	)
	if err != nil {
		logger.Error("Failed to list released episodes for RSS worker", slog.String("error", err.Error()))
	}
	return episodes
}
