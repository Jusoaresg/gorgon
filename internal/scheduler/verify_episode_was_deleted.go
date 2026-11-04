package scheduler

import (
	"github.com/jusoaresg/gorgon/internal/release_guard"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepositoy "github.com/jusoaresg/gorgon/internal/episode/repository"
	epContentRepository "github.com/jusoaresg/gorgon/internal/episode_content/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/utils"
)

func VerifyEpisodeWasDeleted() {
	VerifyEpisodeWasDeletedWithDB(config.GetSQLite())
}

func VerifyEpisodeWasDeletedWithDB(db *sqlx.DB) {
	if db == nil {
		return
	}
	logger := config.GetLogger().WithGroup("scheduler").With("name", "VerifyEpisodeWasDeleted")

	configFile, err := config.LoadConfig()
	if err != nil {
		return
	}

	unlock := release_guard.Lock(db)
	defer unlock()
	episodeRepo := episodeRepositoy.NewEpisodeRepository(db)
	showRepo := showRepository.NewShowRepository(db)
	episodes, err := episodeRepo.ListByTracking(model.TrackingDownloaded)
	if err != nil {
		return
	}

	episodeContentRepo := epContentRepository.NewEpisodeContentRepository(db)

	for _, episode := range episodes {
		if err := release_guard.CheckShow(db, episode.ShowID); err != nil {
			logger.Debug("Skipping protected show reconciliation", "show_id", episode.ShowID, "error", err)
			continue
		}
		if err := release_guard.Check(db, episode.ID, ""); err != nil {
			continue
		}
		contents, err := episodeContentRepo.ListByEpisodeId(episode.ID)
		if err != nil {
			continue
		}

		if contents == nil {
			continue
		}

		show, _ := showRepo.GetById(episode.ShowID)

		for _, episode_content := range contents {
			destExists := false
			if show.Name != "" && configFile.ShowsFolder != "" {
				reconciledPath, recErr := utils.ReconcileEpisodeLink(
					configFile.ShowsFolder,
					configFile.QBittorrentDownloadFolder,
					show.Name,
					show.Name,
					episode,
					episode,
					episode_content,
					configFile.LinkMode,
				)
				if recErr == nil && reconciledPath != "" {
					if episode_content.FilePath != reconciledPath {
						episode_content.FilePath = reconciledPath
						if updateErr := episodeContentRepo.Update(episode_content); updateErr != nil {
							logger.Warn("Unable to update reconciled episode path", "error", updateErr)
							continue
						}
					}
					if _, statErr := os.Stat(reconciledPath); statErr == nil {
						destExists = true
					}
				}
			}

			fileFolder := filepath.Join(configFile.QBittorrentDownloadFolder, episode_content.Name)
			filePath, _ := filepath.Abs(fileFolder)
			_, downloadStatErr := os.Stat(filePath)

			if !destExists && os.IsNotExist(downloadStatErr) {
				// Verification only discards already-absent entries. Physical
				// release deletion belongs to the coordinator; never scan a
				// season folder or guess which remaining link can be removed.
				storedPath, pathErr := utils.ResolveStoredLinkPath(configFile.ShowsFolder, show.Name, episode, episode_content)
				if pathErr != nil {
					logger.Warn("Unable to resolve missing episode link", "error", pathErr)
					continue
				}
				if _, linkErr := os.Lstat(storedPath); !os.IsNotExist(linkErr) {
					if linkErr != nil {
						logger.Warn("Unable to inspect episode link", "error", linkErr)
					}
					continue
				}
				tx, txErr := db.Beginx()
				if txErr != nil {
					logger.Warn("Unable to reconcile missing episode", "error", txErr)
					continue
				}
				if _, txErr = tx.Exec("DELETE FROM episode_contents WHERE id=?", episode_content.ID); txErr == nil {
					var remaining int
					txErr = tx.Get(&remaining, "SELECT count(*) FROM episode_contents WHERE episode_id=?", episode.ID)
					if txErr == nil && remaining == 0 {
						if _, txErr = tx.Exec("DELETE FROM episode_torrents WHERE episode_id=?", episode.ID); txErr == nil {
							_, txErr = tx.Exec("UPDATE episodes SET tracking='skipped' WHERE id=?", episode.ID)
						}
					}
				}
				if txErr == nil {
					txErr = tx.Commit()
				} else {
					_ = tx.Rollback()
				}
				if txErr != nil {
					logger.Warn("Unable to reconcile missing episode", "error", txErr)
					continue
				}

				logger.Info(
					"Episode file not found, setting tracking to skipped",
					slog.Int64("showID", episode.ShowID),
					slog.Int("episodeNumber", episode.Number),
					slog.String("episodeName", episode.Name),
				)
			}
		}
	}
}
