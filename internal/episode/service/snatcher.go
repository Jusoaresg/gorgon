package service

import (
	"context"
	"fmt"
	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/internal/release_deletion"
	"github.com/jusoaresg/gorgon/internal/release_guard"
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
)

var DbWriteMutex sync.Mutex

type UpgradeInfo struct {
	OldScore       int
	NewScore       int
	CutoffScore    int
	OldReleaseName string
}

func SnatchEpisode(episode *model.Episode, response prowlarrSchema.SearchResponse, score ...int) error {
	s := 0
	if len(score) > 0 {
		s = score[0]
	}
	return SnatchEpisodeWithUpgrade(episode, response, s, nil)
}

func SnatchEpisodeWithUpgrade(episode *model.Episode, response prowlarrSchema.SearchResponse, score int, upgrade *UpgradeInfo) error {
	db := config.GetSQLite()
	unlock := release_guard.Lock(db)
	defer unlock()
	return snatchEpisodeLocked(db, episode, response, score, upgrade)
}

func snatchEpisodeLocked(db *sqlx.DB, episode *model.Episode, response prowlarrSchema.SearchResponse, score int, upgrade *UpgradeInfo, prepared ...release_deletion.Operation) error {
	if len(prepared) == 0 {
		if err := validateSnatch(db, episode, response.InfoHash, upgrade != nil); err != nil {
			return err
		}
	}
	logger := config.GetLogger()

	episodeRepo := repository.NewEpisodeRepository(db)
	episodeTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(db)

	var snapshot release_deletion.Operation
	var deletion *release_deletion.Service
	if upgrade != nil && len(prepared) == 0 {
		var err error
		deletion, err = release_deletion.New(db, logger)
		if err != nil {
			return err
		}
		snapshot, err = deletion.PrepareUpgradeSnapshotWhileGuarded(context.Background(), episode.ID, response.InfoHash)
		if err != nil {
			return err
		}
	}
	defer func() {
		if deletion != nil {
			if err := deletion.AbortUpgradeSnapshotWhileGuarded(snapshot); err != nil {
				logger.Error("failed to cancel unstarted upgrade snapshot", "error", err)
			}
		}
	}()
	episode.Tracking = model.TrackingSnatched
	episodeTorrent := episodeTorrentModel.FromSearchResponse(episode.ID, response, score)

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = episodeTorrentRepo.UpsertTx(tx, episodeTorrent); err == nil {
		err = episodeRepo.UpdateTx(tx, *episode)
	}
	if err == nil {
		err = tx.Commit()
	}

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

	tgService, telegramErr := telegramService.NewTelegramService(logger)
	if telegramErr != nil {
		return nil
	}
	showRepo := showRepository.NewShowRepository(db)
	showName := ""
	if show, err := showRepo.GetById(episode.ShowID); err == nil {
		showName = show.Name
	}
	go func() {
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

func validateSnatch(db *sqlx.DB, episode *model.Episode, hash string, upgrading ...bool) error {
	upgrade := len(upgrading) > 0 && upgrading[0]
	if err := release_guard.Check(db, episode.ID, hash); err != nil {
		return err
	}
	current, err := repository.NewEpisodeRepository(db).GetByID(episode.ID)
	if err != nil {
		return err
	}
	if current.Tracking != episode.Tracking || current.Tracking == model.TrackingSkipped || (current.Tracking == model.TrackingDownloaded && !upgrade) || current.Tracking == model.TrackingSnatched {
		return release_guard.ErrStale
	}
	var installed int
	if err = db.Get(&installed, "SELECT count(*) FROM episode_contents WHERE episode_id=?", episode.ID); err != nil {
		return err
	}
	if installed > 0 && !upgrade {
		return fmt.Errorf("existing release requires verified deletion before replacement")
	}
	return nil
}
