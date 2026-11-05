package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/internal/release_deletion"
	"github.com/jusoaresg/gorgon/internal/release_guard"
	"log/slog"
	"os"
	"time"

	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	"github.com/jusoaresg/gorgon/external/qbittorrent/service"
	telegramSchema "github.com/jusoaresg/gorgon/external/telegram/schema"
	telegramService "github.com/jusoaresg/gorgon/external/telegram/service"
	"github.com/jusoaresg/gorgon/internal/episode/events"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	epContentRepository "github.com/jusoaresg/gorgon/internal/episode_content/repository"
	episodeTorrentModel "github.com/jusoaresg/gorgon/internal/episode_torrent/model"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	"github.com/jusoaresg/gorgon/internal/paths"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/utils"
)

type StallEvaluationConfig struct {
	FreshReleaseGracePeriod time.Duration
	MetaDLTimeout           time.Duration
	StalledDLTimeout        time.Duration
	RecentReleaseTTLWindow  time.Duration
	RecentReleaseTTL        time.Duration
}

func DefaultStallConfig() StallEvaluationConfig {
	return StallEvaluationConfig{
		FreshReleaseGracePeriod: 60 * time.Minute,
		MetaDLTimeout:           25 * time.Minute,
		StalledDLTimeout:        60 * time.Minute,
		RecentReleaseTTLWindow:  24 * time.Hour,
		RecentReleaseTTL:        12 * time.Hour,
	}
}

// EvaluateStalledTorrent determines whether an in-progress torrent should be blacklisted due to a download stall,
// honoring a warm-up grace period for newly published RSS releases and tracking real byte progress.
func EvaluateStalledTorrent(
	torrent schema.CheckTorrentResponse,
	epTorrent episodeTorrentModel.EpisodeTorrent,
	cfg StallEvaluationConfig,
	now time.Time,
) (shouldBlacklist bool, reason string, nextStalledSince int64, nextAmountLeft int64, expiresAt *int64) {
	nextAmountLeft = torrent.AmountLeft

	// Client errors (including error and missingFiles) do not establish a bad release.
	// Only download stalls are eligible for blacklisting; other states reset the
	// timer so client downtime cannot count toward a subsequent stall timeout.
	// Check whether real byte progress occurred since last tick.
	isStalledState := torrent.State == "stalledDL" || torrent.State == "metaDL" || torrent.State == "forcedMetaDL"
	madeProgress := epTorrent.LastAmountLeft != -1 && torrent.AmountLeft >= 0 && torrent.AmountLeft < epTorrent.LastAmountLeft

	if !isStalledState || madeProgress {
		return false, "", 0, nextAmountLeft, nil
	}

	// Fresh RSS release protection (swarm warm-up)
	var pubTime time.Time
	hasPubTime := false
	if epTorrent.PublishDate != "" {
		if parsed, err := time.Parse(time.RFC3339, epTorrent.PublishDate); err == nil {
			pubTime = parsed
			hasPubTime = true
		}
	}

	if hasPubTime && now.Sub(pubTime) < cfg.FreshReleaseGracePeriod {
		// Release is brand new on the indexer; keep stall timer frozen at 0 during swarm warm-up
		return false, "", 0, nextAmountLeft, nil
	}

	// Start or continue stall timer
	stalledSince := epTorrent.StalledSince
	if stalledSince == 0 {
		return false, "", now.Unix(), nextAmountLeft, nil
	}

	stalledDuration := now.Sub(time.Unix(stalledSince, 0))

	// Helper to compute optional TTL for releases published within the last 24h
	computeTTL := func() *int64 {
		if hasPubTime && now.Sub(pubTime) <= cfg.RecentReleaseTTLWindow {
			exp := now.Add(cfg.RecentReleaseTTL).Unix()
			return &exp
		}
		return nil
	}

	// Evaluate state-specific timeouts
	if (torrent.State == "metaDL" || torrent.State == "forcedMetaDL") && stalledDuration >= cfg.MetaDLTimeout {
		return true, "metadata download timeout (metaDL)", stalledSince, nextAmountLeft, computeTTL()
	}

	timeout := cfg.StalledDLTimeout
	if torrent.Progress >= 0.50 {
		timeout = timeout * 2
	}

	if torrent.State == "stalledDL" && stalledDuration >= timeout {
		return true, "stalled download timeout (stalledDL)", stalledSince, nextAmountLeft, computeTTL()
	}

	return false, "", stalledSince, nextAmountLeft, nil
}

func ProcessSingleSnatchedDownload(ep *model.Episode, qbittorrentService *service.QBittorrentService) error {
	return ProcessSingleSnatchedDownloadWithDB(config.GetSQLite(), ep, qbittorrentService)
}

func ProcessSingleSnatchedDownloadWithDB(db *sqlx.DB, ep *model.Episode, qbittorrentService *service.QBittorrentService) error {
	unlock := release_guard.Lock(db)
	defer unlock()
	var currentHash string
	if err := db.Get(&currentHash, "SELECT hash FROM episode_torrents WHERE episode_id=?", ep.ID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	cfgForCleanup, err := config.LoadConfig()
	if err != nil {
		return err
	}
	deletion := release_deletion.NewWithOptions(db, config.GetLogger(), release_deletion.Options{Client: qbittorrentService, DownloadRoot: cfgForCleanup.QBittorrentDownloadFolder, ShowsRoot: cfgForCleanup.ShowsFolder})
	upgrading, err := deletion.HasUpgradeSnapshotWhileGuarded(ep.ID, currentHash)
	if err != nil {
		return err
	}
	check := release_guard.Check
	if upgrading {
		check = release_guard.CheckUpgradeImport
	}
	if err := check(db, ep.ID, currentHash); err != nil {
		return err
	}
	current, err := episodeRepository.NewEpisodeRepository(db).GetByID(ep.ID)
	if err != nil {
		return err
	}
	if ep.Tracking != model.TrackingSnatched || current.Tracking != model.TrackingSnatched {
		return release_guard.ErrStale
	}
	fresh := current
	ep = &fresh
	logger := config.GetLogger()
	safeDB := config.GetSafeDB()

	episodeRepo := episodeRepository.NewEpisodeRepository(db)
	episodeContentRepo := epContentRepository.NewEpisodeContentRepository(db)
	episodeTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(db)
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}

	episodeTorrent, err := episodeTorrentRepo.GetByEpisodeID(ep.ID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		contents, listErr := episodeContentRepo.ListByEpisodeId(ep.ID)
		if listErr != nil {
			return listErr
		}
		if len(contents) > 0 {
			return fmt.Errorf("installed release has no torrent association; ownership requires repair")
		}
		return resetToSkipped(ep, episodeRepo)
	}

	var torrentResponse []schema.CheckTorrentResponse
	if err := qbittorrentService.CheckTorrentsWithHash("completed", episodeTorrent.Hash, &torrentResponse); err != nil {
		return err
	}

	if len(torrentResponse) == 0 {
		var anyTorrents []schema.CheckTorrentResponse
		if err := qbittorrentService.CheckTorrentsWithHash("all", episodeTorrent.Hash, &anyTorrents); err != nil {
			return err
		}

		if len(anyTorrents) == 0 {
			// A missing client entry does not prove existing library ownership
			// can be discarded; keep it actionable for coordinated cleanup.
			contents, err := episodeContentRepo.ListByEpisodeId(ep.ID)
			if err != nil {
				return err
			}
			if len(contents) > 0 {
				return fmt.Errorf("torrent is missing but installed release still requires verified cleanup")
			}
			tx, err := db.Beginx()
			if err != nil {
				return err
			}
			defer tx.Rollback()
			if _, err = tx.Exec("UPDATE episodes SET tracking='skipped' WHERE id=?", ep.ID); err != nil {
				return err
			}
			if _, err = tx.Exec("DELETE FROM episode_torrents WHERE episode_id=?", ep.ID); err != nil {
				return err
			}
			if err = tx.Commit(); err != nil {
				return err
			}
			episode.EmitEpisodeTrackingUpdatedEvent(ep.ID, model.TrackingSkipped, "")
			return nil
		}

		activeTorrent := anyTorrents[0]
		now := time.Now()
		shouldBlacklist, reason, nextStalledSince, nextAmountLeft, expiresAt := EvaluateStalledTorrent(
			activeTorrent,
			episodeTorrent,
			DefaultStallConfig(),
			now,
		)

		if shouldBlacklist {
			cfg, err := config.LoadConfig()
			if err != nil {
				return err
			}
			deletion := release_deletion.NewWithOptions(db, logger, release_deletion.Options{Client: qbittorrentService, DownloadRoot: cfg.QBittorrentDownloadFolder, ShowsRoot: cfg.ShowsFolder})
			unlock()
			_, err = deletion.Delete(context.Background(), release_deletion.Request{EpisodeID: ep.ID, Hash: episodeTorrent.Hash, Intention: "blacklist", BlacklistReason: reason, BlacklistExpiresAt: expiresAt})
			return err
		}

		if nextStalledSince != episodeTorrent.StalledSince || nextAmountLeft != episodeTorrent.LastAmountLeft {
			safeDB.Write.Lock()
			updateErr := episodeTorrentRepo.UpdateStallState(ep.ID, nextStalledSince, nextAmountLeft)
			safeDB.Write.Unlock()
			if updateErr != nil {
				return updateErr
			}
		}

		logger.Debug(fmt.Sprintf("Episode S%02d E%02d %s still downloading (state=%s).", ep.Season, ep.Number, ep.Name, activeTorrent.State))
		return nil
	}
	torrent := torrentResponse[0]

	if torrent.Hash == episodeTorrent.Hash {
		logger.Info(fmt.Sprintf("Episode S%02d E%02d %s found - Torrent: %s", ep.Season, ep.Number, ep.Name, torrent.Name))

		safeDB.Write.Lock()
		defer safeDB.Write.Unlock()

		ep.Tracking = model.TrackingDownloaded

		contents, err := qbittorrentService.GetContent(torrent.Hash)
		if err != nil {
			return err
		}

		showRepo := showRepository.NewShowRepository(db)
		show, err := showRepo.GetById(ep.ShowID)
		if err != nil {
			return err
		}

		existingContents, err := episodeContentRepo.ListByEpisodeId(ep.ID)
		if err != nil {
			return err
		}
		// The snatcher historically overwrote the old hash. Without its old
		// manifest an upgrade cannot safely delete existing release artifacts.
		if len(existingContents) > 0 && !upgrading {
			return fmt.Errorf("existing release requires verified cleanup before import")
		}
		wasUpgrade := upgrading
		if upgrading {
			if _, err := deletion.CompleteUpgradeSnapshotWhileGuarded(context.Background(), ep.ID, torrent.Hash); err != nil {
				return err
			}
		}

		tx, err := db.Beginx()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := episodeRepo.UpdateTx(tx, *ep); err != nil {
			return err
		}

		var createdLinks []string
		for _, content := range contents {
			destPath, err := utils.SymlinkPathForEpisode(cfg.ShowsFolder, show.Name, *ep, content)
			if err != nil {
				cleanupErr := removeCreatedLinks(createdLinks)
				tx.Rollback()
				return errors.Join(err, cleanupErr)
			}
			episodeDownloadFile, err := paths.GetEpisodeDownloadFile(content.Name)
			if err != nil {
				cleanupErr := removeCreatedLinks(createdLinks)
				tx.Rollback()
				return errors.Join(err, cleanupErr)
			}

			if _, statErr := os.Lstat(destPath); statErr == nil {
				return rollbackCreatedLinks(createdLinks, fmt.Errorf("import destination already exists"))
			} else if !os.IsNotExist(statErr) {
				return rollbackCreatedLinks(createdLinks, statErr)
			}
			if err := utils.CreateMediaLink(episodeDownloadFile, destPath, cfg.LinkMode); err != nil {
				logger.Error("Failed to create media link",
					slog.String("mode", cfg.LinkMode),
					slog.String("from", episodeDownloadFile),
					slog.String("to", destPath),
					slog.String("error", err.Error()),
				)
				cleanupErr := removeCreatedLinks(createdLinks)
				tx.Rollback()
				return errors.Join(err, cleanupErr)
			}
			createdLinks = append(createdLinks, destPath)

			content.FilePath = destPath
			content.EpisodeId = ep.ID
			if _, err := episodeContentRepo.CreateTx(tx, content); err != nil {
				cleanupErr := removeCreatedLinks(createdLinks)
				return errors.Join(err, cleanupErr)
			}
		}

		if err := tx.Commit(); err != nil {
			cleanupErr := removeCreatedLinks(createdLinks)
			return errors.Join(err, cleanupErr)
		}

		episode.EmitEpisodeTrackingUpdatedEvent(ep.ID, model.TrackingDownloaded, episodeTorrent.InfoUrl)

		tgService, telegramErr := telegramService.NewTelegramService(logger)
		if telegramErr != nil {
			return nil
		}
		go func() {
			if err := tgService.SendEpisodeDownloaded(telegramSchema.EpisodeDownloadedTemplateInput{
				ShowName:     show.Name,
				Season:       ep.Season,
				Episode:      ep.Number,
				EpisodeTitle: ep.Name,
				IsUpgrade:    wasUpgrade,
				Score:        episodeTorrent.Score,
				ReleaseTitle: episodeTorrent.Title,
			}); err != nil {
				logger.Error("failed to send telegram episode downloaded notification", slog.String("error", err.Error()))
			}
		}()

		return nil
	}
	logger.Info(fmt.Sprintf("Episode S%02d E%02d %s not found between the progress torrents.", ep.Season, ep.Number, ep.Name))
	return nil
}

func resetToSkipped(ep *model.Episode, episodeRepo *episodeRepository.EpisodeRepository) error {
	logger := config.GetLogger()
	safeDB := config.GetSafeDB()

	safeDB.Write.Lock()
	defer safeDB.Write.Unlock()

	ep.Tracking = model.TrackingSkipped

	if err := episodeRepo.Update(*ep); err != nil {
		logger.Error("failed to reset episode tracking to skipped", slog.Int64("episode_id", ep.ID), slog.String("error", err.Error()))
		return err
	}

	episode.EmitEpisodeTrackingUpdatedEvent(ep.ID, model.TrackingSkipped, "")

	return nil
}

// Only paths successfully created by this import attempt enter this list.
func removeCreatedLinks(paths []string) error {
	var result error
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			result = errors.Join(result, err)
		}
	}
	return result
}
func rollbackCreatedLinks(paths []string, cause error) error {
	return errors.Join(cause, removeCreatedLinks(paths))
}
