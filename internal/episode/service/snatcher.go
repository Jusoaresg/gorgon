package service

import (
	"log/slog"
	"sync"

	"github.com/jusoaresg/gorgon/config"
	prowlarrSchema "github.com/jusoaresg/gorgon/external/prowlarr/schema"
	telegramSchema "github.com/jusoaresg/gorgon/external/telegram/schema"
	telegramService "github.com/jusoaresg/gorgon/external/telegram/service"
	episodeEvents "github.com/jusoaresg/gorgon/internal/episode/events"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	"github.com/jusoaresg/gorgon/internal/episode/repository"
	episodeTorrentModel "github.com/jusoaresg/gorgon/internal/episode_torrent/model"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/pkg/concurrency"
)

var DbWriteMutex sync.Mutex

func SnatchEpisode(episode *model.Episode, response prowlarrSchema.SearchResponse) error {
	logger := config.GetLogger()
	safeDB := config.GetSafeDB()

	episodeRepo := repository.NewEpisodeRepository(safeDB.Db)
	episodeTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(safeDB.Db)

	episode.Tracking = model.TrackingSnatched
	episodeTorrent := episodeTorrentModel.FromSearchResponse(episode.ID, response)

	err := concurrency.WithWriteLock(safeDB.Write, func() error {
		if _, err := episodeTorrentRepo.Upsert(episodeTorrent); err != nil {
			return err
		}
		return episodeRepo.Update(*episode)
	})
	if err != nil {
		logger.Error(
			"failed to update episode status to snatched",
			slog.Int64("episode_id", episode.ID),
			slog.Int64("show_id", episode.ShowID),
			slog.String("torrent_hash", response.InfoHash),
		)
		return err
	}

	episodeEvents.EmitEpisodeTrackingUpdatedEvent(episode.ID, model.TrackingSnatched, episodeTorrent.InfoUrl)

	go func() {
		tgService, err := telegramService.NewTelegramService(logger)
		if err != nil {
			return
		}
		showRepo := showRepository.NewShowRepository(safeDB.Db)
		showName := ""
		if show, err := showRepo.GetById(episode.ShowID); err == nil {
			showName = show.Name
		}
		if err := tgService.SendEpisodeSnatched(telegramSchema.EpisodeSnatchedTemplateInput{
			ShowName:     showName,
			Season:       episode.Season,
			Episode:      episode.Number,
			EpisodeTitle: episode.Name,
			ReleaseTitle: response.Title,
			ReleaseURL:   episodeTorrent.InfoUrl,
			Indexer:      response.Indexer,
		}); err != nil {
			logger.Error("failed to send telegram episode snatched notification", slog.String("error", err.Error()))
		}
	}()

	return nil
}
