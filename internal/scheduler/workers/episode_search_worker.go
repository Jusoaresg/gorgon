package workers

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jusoaresg/gorgon/config"
	prowlarr "github.com/jusoaresg/gorgon/external/prowlarr/service"
	qbittorrent "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	"github.com/jusoaresg/gorgon/internal/episode/repository"
)

func StartEpisodeSearchWorker(ctx context.Context, workerCount int, prowlarrService *prowlarr.ProwlarrSearchService, qbittorrentService *qbittorrent.QBittorrentService) {
	logger := config.GetLogger().WithGroup("worker").With("name", "episodeSync")

	episodeChan := make(chan model.Episode, 50)
	var wg sync.WaitGroup

	logger.Info("Starting episode sync workers", slog.Int("count", workerCount))

	for i := range workerCount {
		workerID := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("worker started", slog.Int("worker_id", workerID))
			processEpisodesWorker(ctx, episodeChan, prowlarrService, qbittorrentService)
		}()
	}

	ticker := time.NewTicker(15 * time.Minute)
	defer func() {
		ticker.Stop()
		close(episodeChan)
		wg.Wait()
		logger.Info("Episode sync workers stopped cleanly")
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logger.Info("Checking for backfill wanted episodes")

			episodes := fetchEpisodeSearchWantedEpisodes()
			if len(episodes) == 0 {
				logger.Info("No wanted or missing episodes due for backfill search")
				continue
			}

			for _, ep := range episodes {
				logger.Info("Queuing episode for backfill search",
					slog.Int("episodeID", int(ep.ID)),
					slog.Int64("showID", ep.ShowID),
					slog.String("name", ep.Name),
				)
				select {
				case <-ctx.Done():
					return
				case episodeChan <- ep:
				}
			}
		}
	}
}

const (
	backfillRecentThreshold = 48 * 3600 // 48 hours
	backfillRecentCooldown  = 30 * 60   // 30 minutes cooldown for recent episodes
	backfillOldCooldown     = 6 * 3600  // 6 hours cooldown for older backlog episodes
	backfillBatchLimit      = 25        // max episodes searched per cycle
)

func fetchEpisodeSearchWantedEpisodes() []model.Episode {
	logger := config.GetLogger().WithGroup("worker").With("name", "episodeSync")
	db := config.GetSQLite()

	episodeRepo := repository.NewEpisodeRepository(db)

	episodes, err := episodeRepo.ListBackfillCandidates(
		time.Now().Unix(),
		backfillRecentThreshold,
		backfillRecentCooldown,
		backfillOldCooldown,
		backfillBatchLimit,
	)
	if err != nil {
		logger.Error("Failed to list backfill candidates", slog.String("error", err.Error()))
		return nil
	}

	return episodes
}
