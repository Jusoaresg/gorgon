package service

import (
	"log/slog"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
	qbittorrentService "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	epContentModel "github.com/jusoaresg/gorgon/internal/episode_content/model"
	epContentRepository "github.com/jusoaresg/gorgon/internal/episode_content/repository"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	"github.com/jusoaresg/gorgon/internal/paths"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/utils"
)

type EpisodeDownloaderInterface interface {
	DownloadEpisode(ep model.Episode, response schema.SearchResponse, score ...int) error
	DownloadEpisodeTorrent(ep model.Episode, response schema.SearchResponse) error
	UpgradeEpisode(ep model.Episode, response schema.SearchResponse, newScore int) error
}

type EpisodeDownloader struct {
	db *sqlx.DB
}

func NewEpisodeDownloader(db ...*sqlx.DB) *EpisodeDownloader {
	var d *sqlx.DB
	if len(db) > 0 {
		d = db[0]
	}
	return &EpisodeDownloader{db: d}
}

func (d *EpisodeDownloader) getDB() *sqlx.DB {
	if d != nil && d.db != nil {
		return d.db
	}
	return config.GetSQLite()
}

func (d *EpisodeDownloader) DownloadEpisode(ep model.Episode, response schema.SearchResponse, score ...int) error {
	logger := config.GetLogger()

	torrentService, err := qbittorrentService.NewQBittorrentService(logger)
	if err != nil {
		return err
	}

	if err := torrentService.AddTorrent(response.Guid); err != nil {
		logger.Error("failed to add torrent", slog.String("error", err.Error()))
		return err
	}

	logger.Info("added torrent to torrent client")

	if err := SnatchEpisode(&ep, response, score...); err != nil {
		if err := torrentService.DeleteTorrent(response.InfoHash, true); err != nil {
			return err
		}
		return err
	}

	return nil
}

func (d *EpisodeDownloader) DownloadEpisodeTorrent(ep model.Episode, response schema.SearchResponse) error {
	return d.DownloadEpisode(ep, response)
}

func (d *EpisodeDownloader) UpgradeEpisode(ep model.Episode, response schema.SearchResponse, newScore int) error {
	logger := config.GetLogger()
	db := d.getDB()

	torrentService, err := qbittorrentService.NewQBittorrentService(logger)
	if err != nil {
		return err
	}

	epTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(db)
	epContentRepo := epContentRepository.NewEpisodeContentRepository(db)
	showRepo := showRepository.NewShowRepository(db)

	cfg, _ := config.LoadConfig()

	// 1. Remove old torrent from client and clean old files/symlinks
	oldTorrent, err := epTorrentRepo.GetByEpisodeID(ep.ID)
	if err == nil && oldTorrent.Hash != "" {
		if err := torrentService.DeleteTorrent(oldTorrent.Hash, true); err != nil {
			logger.Warn("failed to delete old torrent from client during upgrade",
				slog.String("hash", oldTorrent.Hash),
				slog.String("error", err.Error()),
			)
		}
	}

	var showName string
	show, err := showRepo.GetById(ep.ShowID)
	if err == nil {
		showName = show.Name
	}

	downloadFolder, _ := paths.GetTorrentDownloadFolder()
	contents, _ := epContentRepo.ListByEpisodeId(ep.ID)
	for _, content := range contents {
		if downloadFolder != "" {
			_, _ = utils.DeleteFile(downloadFolder, content.Name)
		}
		if cfg != nil && showName != "" {
			_ = utils.DeleteSymlink(cfg.ShowsFolder, showName, ep, content)
		}
		_ = epContentRepo.DeleteById(content.ID)
	}

	if cfg != nil && showName != "" {
		_ = utils.DeleteSymlink(cfg.ShowsFolder, showName, ep, epContentModel.EpisodeContent{})
		_ = utils.CleanBrokenSymlinksInSeason(cfg.ShowsFolder, showName, ep.Season)
	}

	// 2. Add new torrent to client
	if err := torrentService.AddTorrent(response.Guid); err != nil {
		logger.Error("failed to add upgrade torrent to client", slog.String("error", err.Error()))
		return err
	}

	logger.Info("added upgrade torrent to torrent client",
		slog.String("title", response.Title),
		slog.Int("new_score", newScore),
	)

	// 3. Update database record with new score and snatch status
	if err := SnatchEpisode(&ep, response, newScore); err != nil {
		_ = torrentService.DeleteTorrent(response.InfoHash, true)
		return err
	}

	return nil
}
