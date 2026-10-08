package workers

import (
	"context"
	"log/slog"
	"time"

	"github.com/jusoaresg/gorgon/config"
	prowlarr "github.com/jusoaresg/gorgon/external/prowlarr/service"
	qbittorrent "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	"github.com/jusoaresg/gorgon/internal/scheduler/jobs"
	"github.com/jusoaresg/gorgon/pkg/services"
)

func processEpisodesWorker(ctx context.Context, episodesChan <-chan model.Episode, prowlarrService *prowlarr.ProwlarrSearchService, qbittorrentService *qbittorrent.QBittorrentService) {
	logger := config.GetLogger().WithGroup("worker").With("name", "processEpisodeWorker")

	if prowlarrService != nil || qbittorrentService != nil {
		for {
			errs := services.CheckAllConnections(prowlarrService, qbittorrentService)
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

	episodeLogger := logger.WithGroup("episode")
	for {
		select {
		case <-ctx.Done():
			return
		case episode, ok := <-episodesChan:
			if !ok {
				return
			}
			episodeLogger.Info("Start processing episode",
				slog.Int("episodeID", int(episode.ID)),
				slog.Int64("showID", episode.ShowID),
				slog.String("episodeName", episode.Name),
			)
			err := jobs.ProcessBackfillSingleEpisode(&episode, prowlarrService, qbittorrentService)
			if err != nil {
				episodeLogger.Error(
					"Error processing single episode",
					slog.Int("episodeID", int(episode.ID)),
					slog.Int64("showID", episode.ShowID),
					slog.String("episodeName", episode.Name),
					slog.String("error", err.Error()))
			} else {
				episodeLogger.Info("Finished processing episode",
					slog.Int("episodeID", int(episode.ID)),
					slog.Int64("showID", episode.ShowID),
					slog.String("episodeName", episode.Name),
				)
			}
		}
	}
}
