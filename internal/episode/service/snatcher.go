package service

import (
	"log/slog"
	"strings"
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

type UpgradeInfo struct {
	OldScore       int
	NewScore       int
	CutoffScore    int
	OldReleaseName string
	IsManual       bool
}

func SnatchEpisode(episode *model.Episode, response prowlarrSchema.SearchResponse, score ...int) error {
	s := 0
	if len(score) > 0 {
		s = score[0]
	}
	return SnatchEpisodeWithOptions(episode, response, s, nil, false)
}

func SnatchEpisodeManual(episode *model.Episode, response prowlarrSchema.SearchResponse, score int) error {
	return SnatchEpisodeWithOptions(episode, response, score, nil, true)
}

func SnatchEpisodeWithUpgrade(episode *model.Episode, response prowlarrSchema.SearchResponse, score int, upgrade *UpgradeInfo) error {
	isManual := upgrade != nil && upgrade.IsManual
	return SnatchEpisodeWithOptions(episode, response, score, upgrade, isManual)
}

func SnatchEpisodeWithOptions(episode *model.Episode, response prowlarrSchema.SearchResponse, score int, upgrade *UpgradeInfo, isManual bool) error {
	logger := config.GetLogger()
	safeDB := config.GetSafeDB()

	episodeRepo := repository.NewEpisodeRepository(safeDB.Db)
	episodeTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(safeDB.Db)

	prevTracking := episode.Tracking
	episode.Tracking = model.TrackingSnatched
	episodeTorrent := episodeTorrentModel.FromSearchResponse(episode.ID, response, score)
	episodeTorrent.IsManual = isManual

	err := concurrency.WithWriteLock(safeDB.Write, func() error {
		if prevTracking == model.TrackingDownloaded || prevTracking == model.TrackingSnatched {
			if existing, err := episodeTorrentRepo.GetByEpisodeID(episode.ID); err == nil {
				if prevTracking == model.TrackingDownloaded && existing.Hash != "" && !strings.EqualFold(existing.Hash, response.InfoHash) {
					episodeTorrent.SnapshotPreviousFrom(existing)
				} else if prevTracking == model.TrackingSnatched && existing.HasPrevious() {
					episodeTorrent.CopyPreviousFrom(existing)
				}
			}
		}

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
		input := telegramSchema.EpisodeSnatchedTemplateInput{
			EpisodeID:    episode.ID,
			ShowName:     showName,
			Season:       episode.Season,
			Episode:      episode.Number,
			EpisodeTitle: episode.Name,
			ReleaseTitle: response.Title,
			ReleaseURL:   episodeTorrent.InfoUrl,
			Indexer:      response.Indexer,
			Score:        score,
		}
		if upgrade != nil {
			input.IsUpgrade = true
			input.OldScore = upgrade.OldScore
			input.CutoffScore = upgrade.CutoffScore
			input.OldReleaseName = upgrade.OldReleaseName
		}
		if err := tgService.SendEpisodeSnatched(input); err != nil {
			logger.Error("failed to send telegram episode snatched notification", slog.String("error", err.Error()))
		}
	}()

	return nil
}
