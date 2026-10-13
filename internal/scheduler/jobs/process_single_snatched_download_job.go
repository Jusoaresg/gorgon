package jobs

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	"github.com/jusoaresg/gorgon/external/qbittorrent/service"
	telegramSchema "github.com/jusoaresg/gorgon/external/telegram/schema"
	telegramService "github.com/jusoaresg/gorgon/external/telegram/service"
	blacklistModel "github.com/jusoaresg/gorgon/internal/blacklist/model"
	blacklistRepository "github.com/jusoaresg/gorgon/internal/blacklist/repository"
	"github.com/jusoaresg/gorgon/internal/episode/events"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	episodeService "github.com/jusoaresg/gorgon/internal/episode/service"
	epContentModel "github.com/jusoaresg/gorgon/internal/episode_content/model"
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

// EvaluateStalledTorrent determines whether an in-progress torrent should be blacklisted due to stall or error,
// honoring a warm-up grace period for newly published RSS releases and tracking real byte progress.
func EvaluateStalledTorrent(
	torrent schema.CheckTorrentResponse,
	epTorrent episodeTorrentModel.EpisodeTorrent,
	cfg StallEvaluationConfig,
	now time.Time,
) (shouldBlacklist bool, reason string, nextStalledSince int64, nextAmountLeft int64, expiresAt *int64) {
	nextAmountLeft = torrent.AmountLeft

	// 1. Immediate failure states from qBittorrent
	if torrent.State == "error" || torrent.State == "missingFiles" {
		return true, "qBittorrent state: " + torrent.State, epTorrent.StalledSince, nextAmountLeft, nil
	}

	// 2. Check if state is stalled and whether real byte progress occurred since last tick
	isStalledState := torrent.State == "stalledDL" || torrent.State == "metaDL" || torrent.State == "forcedMetaDL"
	madeProgress := epTorrent.LastAmountLeft != -1 && torrent.AmountLeft >= 0 && torrent.AmountLeft < epTorrent.LastAmountLeft

	if !isStalledState || madeProgress {
		return false, "", 0, nextAmountLeft, nil
	}

	// 3. Fresh RSS release protection (swarm warm-up)
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

	// 4. Start or continue stall timer
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

	// 5. Evaluate state-specific timeouts
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
	logger := config.GetLogger()
	safeDB := config.GetSafeDB()

	episodeRepo := episodeRepository.NewEpisodeRepository(safeDB.Db)
	episodeContentRepo := epContentRepository.NewEpisodeContentRepository(safeDB.Db)
	episodeTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(safeDB.Db)
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}

	episodeTorrent, err := episodeTorrentRepo.GetByEpisodeID(ep.ID)
	if err != nil {
		logger.Info(fmt.Sprintf("Episode S%02d E%02d %s has no torrent associated, resetting to skipped.", ep.Season, ep.Number, ep.Name))
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
			logger.Info(fmt.Sprintf("Episode S%02d E%02d %s no longer in torrent client, resetting to skipped.", ep.Season, ep.Number, ep.Name))

			if err := resetToSkipped(ep, episodeRepo); err != nil {
				return err
			}

			contents, err := episodeContentRepo.ListByEpisodeId(ep.ID)
			if err == nil {
				showRepo := showRepository.NewShowRepository(safeDB.Db)
				show, err := showRepo.GetById(ep.ShowID)
				if err == nil {
					for _, content := range contents {
						_ = utils.DeleteSymlink(cfg.ShowsFolder, show.Name, *ep, content)
						_ = episodeContentRepo.DeleteById(content.ID)
					}
					_ = utils.DeleteSymlink(cfg.ShowsFolder, show.Name, *ep, epContentModel.EpisodeContent{})
					_ = utils.CleanBrokenSymlinksInSeason(cfg.ShowsFolder, show.Name, ep.Season)
				}
			}

			if err := episodeTorrentRepo.DeleteByEpisodeID(ep.ID); err != nil {
				logger.Error("failed to delete episode torrent", slog.Int64("episode_id", ep.ID), slog.String("error", err.Error()))
				return err
			}

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
			logger.Warn(
				"Blacklisting stalled or failed torrent and triggering re-search",
				slog.Int64("episode_id", ep.ID),
				slog.Int64("show_id", ep.ShowID),
				slog.String("hash", episodeTorrent.Hash),
				slog.String("title", episodeTorrent.Title),
				slog.String("state", activeTorrent.State),
				slog.String("reason", reason),
			)

			_ = qbittorrentService.DeleteTorrent(episodeTorrent.Hash, true)

			releaseTitle := episodeTorrent.Title
			if releaseTitle == "" {
				releaseTitle = activeTorrent.Name
			}

			safeDB.Write.Lock()
			blRepo := blacklistRepository.NewBlacklistRepository(safeDB.Db)
			epIDCopy := ep.ID
			_, _ = blRepo.Create(blacklistModel.ReleaseBlacklist{
				ShowID:       ep.ShowID,
				EpisodeID:    &epIDCopy,
				InfoHash:     episodeTorrent.Hash,
				ReleaseTitle: releaseTitle,
				Indexer:      episodeTorrent.Indexer,
				Reason:       reason,
				CreatedAt:    now.Unix(),
				ExpiresAt:    expiresAt,
			})
			_ = episodeTorrentRepo.DeleteByEpisodeID(ep.ID)
			ep.Tracking = model.TrackingWanted
			_ = episodeRepo.Update(*ep)
			safeDB.Write.Unlock()

			episode.EmitEpisodeTrackingUpdatedEvent(ep.ID, model.TrackingWanted, "")

			showRepo := showRepository.NewShowRepository(safeDB.Db)
			searchSvc := episodeService.NewEpisodeSearchService(
				safeDB.Db,
				logger,
				&episodeService.EpisodeSearcher{},
				episodeService.NewEpisodeDownloader(safeDB.Db),
				episodeRepo,
				showRepo,
			)
			go func(episodeID int) {
				if err := searchSvc.ProcessSingleEpisode(episodeID); err != nil {
					logger.Error("Failed to re-search episode after blacklisting stalled torrent",
						slog.Int("episode_id", episodeID),
						slog.String("error", err.Error()),
					)
				}
			}(int(ep.ID))

			return nil
		}

		if nextStalledSince != episodeTorrent.StalledSince || nextAmountLeft != episodeTorrent.LastAmountLeft {
			safeDB.Write.Lock()
			_ = episodeTorrentRepo.UpdateStallState(ep.ID, nextStalledSince, nextAmountLeft)
			safeDB.Write.Unlock()
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

		showRepo := showRepository.NewShowRepository(config.GetSQLite())
		show, err := showRepo.GetById(ep.ShowID)

		// Atomic cleanup of prior files/symlinks if this download is an upgrade
		existingContents, _ := episodeContentRepo.ListByEpisodeId(ep.ID)
		downloadFolder, _ := paths.GetTorrentDownloadFolder()
		for _, ec := range existingContents {
			isSameFile := false
			for _, newContent := range contents {
				if newContent.Name == ec.Name {
					isSameFile = true
					break
				}
			}
			if !isSameFile && downloadFolder != "" && ec.Name != "" {
				_, _ = utils.DeleteFile(downloadFolder, ec.Name)
			}
			if cfg != nil && show.Name != "" {
				_ = utils.DeleteSymlink(cfg.ShowsFolder, show.Name, *ep, ec)
			}
			_ = episodeContentRepo.DeleteById(ec.ID)
		}
		if cfg != nil && show.Name != "" {
			_ = utils.DeleteSymlink(cfg.ShowsFolder, show.Name, *ep, epContentModel.EpisodeContent{})
			_ = utils.CleanBrokenSymlinksInSeason(cfg.ShowsFolder, show.Name, ep.Season)
		}

		tx, err := config.GetSQLite().Beginx()
		if err != nil {
			return err
		}
		episodeRepo.UpdateTx(tx, *ep)
		for _, content := range contents {
			content.FilePath = torrent.SavePath
			content.EpisodeId = ep.ID
			if _, err := episodeContentRepo.CreateTx(tx, content); err != nil {
				return err
			}

			symlinkPath, err := utils.SymlinkPathForEpisode(cfg.ShowsFolder, show.Name, *ep, content)
			if err != nil {
				tx.Rollback()
				return err
			}
			episodeDownloadFolder, err := paths.GetEpisodeDownloadFile(content.Name)
			if err != nil {
				tx.Rollback()
				return err
			}

			if err := utils.CreateSymlink(episodeDownloadFolder, symlinkPath); err != nil {
				logger.Error("Failed to create symlink", slog.String("from", episodeDownloadFolder), slog.String("to", symlinkPath), slog.String("error", err.Error()))
			}
		}

		if err := tx.Commit(); err != nil {
			return err
		}

		episode.EmitEpisodeTrackingUpdatedEvent(ep.ID, model.TrackingDownloaded, episodeTorrent.InfoUrl)

		go func() {
			tgService, err := telegramService.NewTelegramService(logger)
			if err != nil {
				return
			}
			if err := tgService.SendEpisodeDownloaded(telegramSchema.EpisodeDownloadedTemplateInput{
				ShowName:     show.Name,
				Season:       ep.Season,
				Episode:      ep.Number,
				EpisodeTitle: ep.Name,
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
