package scheduler

import (
	"log/slog"
	"os"
	"path/filepath"

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
	logger := config.GetLogger().WithGroup("scheduler").With("name", "VerifyEpisodeWasDeleted")
	db := config.GetSQLite()

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

		show, err := showRepo.GetById(episode.ShowID)

		for _, episode_content := range contents {

			fileFolder := filepath.Join(configFile.QBittorrentDownloadFolder, episode_content.Name)
			filePath, _ := filepath.Abs(fileFolder)

			_, err := os.Stat(filePath)
			if os.IsNotExist(err) {
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

				episodeContentRepo.DeleteById(episode_content.ID)

				episodeTorrentRepo.DeleteByEpisodeID(episode.ID)

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
