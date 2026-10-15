package scheduler

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepositoy "github.com/jusoaresg/gorgon/internal/episode/repository"
	epContentModel "github.com/jusoaresg/gorgon/internal/episode_content/model"
	epContentRepository "github.com/jusoaresg/gorgon/internal/episode_content/repository"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
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

	episodeRepo := episodeRepositoy.NewEpisodeRepository(db)
	episodeTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(db)
	showRepo := showRepository.NewShowRepository(db)
	episodes, err := episodeRepo.ListByTracking(model.TrackingDownloaded)
	if err != nil {
		return
	}

	episodeContentRepo := epContentRepository.NewEpisodeContentRepository(db)

	for _, episode := range episodes {
		contents, err := episodeContentRepo.ListByEpisodeId(episode.ID)
		if err != nil {
			continue
		}

		if contents == nil {
			continue
		}

		show, _ := showRepo.GetById(episode.ShowID)
		if show.Name != "" && configFile.ShowsFolder != "" {
			_ = utils.MigrateShowFolder(configFile.ShowsFolder, show.Name, show.Name)
		}

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
						_ = episodeContentRepo.Update(episode_content)
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
				episode.SetNotInstalled()

				err := episodeRepo.Update(episode)
				if err != nil {
					continue
				}

				if show.Name != "" {
					_ = utils.DeleteSymlink(configFile.ShowsFolder, show.Name, episode, episode_content)
					_ = utils.DeleteSymlink(configFile.ShowsFolder, show.Name, episode, epContentModel.EpisodeContent{})
					_ = utils.CleanBrokenSymlinksInSeason(configFile.ShowsFolder, show.Name, episode.Season)
				}

				_ = episodeContentRepo.DeleteById(episode_content.ID)
				_ = episodeTorrentRepo.DeleteByEpisodeID(episode.ID)

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
