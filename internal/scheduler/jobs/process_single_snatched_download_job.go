package jobs

import (
	"fmt"
	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	"github.com/jusoaresg/gorgon/external/qbittorrent/service"
	"github.com/jusoaresg/gorgon/internal/episode/events"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	epContentModel "github.com/jusoaresg/gorgon/internal/episode_content/model"
	epContentRepository "github.com/jusoaresg/gorgon/internal/episode_content/repository"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	"github.com/jusoaresg/gorgon/internal/paths"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/utils"
	"log/slog"
)

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

		logger.Info(fmt.Sprintf("Episode S%02d E%02d %s not found between the completed torrents.", ep.Season, ep.Number, ep.Name))
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
