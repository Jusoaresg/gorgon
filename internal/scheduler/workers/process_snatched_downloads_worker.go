package workers

import (
	"context"
	"log/slog"
	"time"

	"github.com/jusoaresg/gorgon/config"
	qbittorrent "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	"github.com/jusoaresg/gorgon/internal/scheduler/jobs"
	"github.com/jusoaresg/gorgon/pkg/services"
)

func processSnatchedDownloadsWorker(ctx context.Context, episodesChan <-chan model.Episode, qbittorrentService *qbittorrent.QBittorrentService) {
	logger := config.GetLogger().WithGroup("worker").With("name", "processSnatchedDownloadsWorker")

	if qbittorrentService != nil {
		for {
			errs := services.CheckAllConnections(qbittorrentService)
			if len(errs) > 0 {
				logger.Error("Failed to connect to one or more services", slog.Any("errors", errs))
				select {
				case <-ctx.Done():
					return
				case <-time.After(30 * time.Second):
					continue
				}
			}
			break
		}
	}

	episodeLogger := logger.WithGroup("episode").With("source", "snatched")
	for {
		select {
		case <-ctx.Done():
			return
		case episode, ok := <-episodesChan:
			if !ok {
				return
			}
			err := jobs.ProcessSingleSnatchedDownload(&episode, qbittorrentService)
			if err != nil {
				episodeLogger.Error(
					"Error processing snatched episode downloaded",
					slog.Int("episodeID", int(episode.ID)),
					slog.Int64("showID", episode.ShowID),
					slog.String("episodeName", episode.Name),
					slog.String("error", err.Error()))
			}
		}
	}
}
