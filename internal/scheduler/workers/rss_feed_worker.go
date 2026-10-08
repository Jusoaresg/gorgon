package workers

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
	"github.com/jusoaresg/gorgon/external/prowlarr/service"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	"github.com/jusoaresg/gorgon/internal/episode/repository"
)

// NOTE: Responses could be a pointer
type EpisodeJob struct {
	Episode   model.Episode
	Responses []schema.SearchResponse
}

func StartRssEpisodeFetcherWorker(ctx context.Context, workerCount int, prowlarrService *service.ProwlarrSearchService) {
	logger := config.GetLogger().WithGroup("worker").With("name", "StartRssFeedWorker")
	rssProcessor := NewRssReleaseProcessor(config.GetSQLite())

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

	ticker := time.NewTicker(time.Minute * 1)
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
			logger.Info("checking for wanted episodes")

			episodes := fetchRssFeedWantedEpisodes()
			if len(episodes) == 0 {
				logger.Info("no episodes found with status 'wanted' or 'missing'")
				continue
			}

			var currentResponses []schema.SearchResponse

			//TODO: Required words here
			query := strings.Join([]string{"multi subs"}, " ")
			prowlarrService.Search(&schema.SearchRequest{Query: query}, &currentResponses)

			for _, ep := range episodes {
				logger.Info(
					"queuing episode for processing",
					slog.Int64("episode_id", ep.ID),
					slog.Int64("show_id", ep.ShowID),
					slog.String("name", ep.Name),
				)
				select {
				case <-ctx.Done():
					return
				case jobChan <- EpisodeJob{
					Episode:   ep,
					Responses: currentResponses,
				}:
				}
			}
		}
	}
}

func fetchRssFeedWantedEpisodes() []model.Episode {
	logger := config.GetLogger().WithGroup("worker").With("name", "fetchWantedEps")
	db := config.GetSQLite()

	episodeRepositoy := repository.NewEpisodeRepository(db)

	episodes, err := episodeRepositoy.ListByTracking("wanted", "missing")
	if err != nil {
		logger.Error("Failed to list episodes by tracking", slog.String("error", err.Error()))
	}
	return episodes
}
