package workers

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jusoaresg/gorgon/config"
	qbittorrent "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	"github.com/jusoaresg/gorgon/internal/episode/repository"
)

func VerifySnatchedDownloadsWorker(ctx context.Context, workerCount int, qbittorrentService *qbittorrent.QBittorrentService) {
	episodeChan := make(chan model.Episode, 100)
	var wg sync.WaitGroup

	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			processSnatchedDownloadsWorker(ctx, episodeChan, qbittorrentService)
		}()
	}

	ticker := time.NewTicker(time.Second * 30)
	defer func() {
		ticker.Stop()
		close(episodeChan)
		wg.Wait()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			episodes := fetchSnatchedEpisodes()
			if len(episodes) == 0 {
				continue
			}

			for _, ep := range episodes {
				select {
				case <-ctx.Done():
					return
				case episodeChan <- ep:
				}
			}
		}
	}
}

func fetchSnatchedEpisodes() []model.Episode {
	logger := config.GetLogger()
	episodeRepo := repository.NewEpisodeRepository(config.GetSQLite())

	//TODO: Put an limit of 100 here
	episodes, err := episodeRepo.ListByTracking(model.TrackingSnatched)
	if err != nil {
		logger.Error(
			"Error fetching snatched episodes",
			slog.Any("Episodes", episodes),
			slog.String("worker", "fetchSnatchedEpisodes"),
			slog.String("error", err.Error()))
		return nil
	}

	return episodes
}
