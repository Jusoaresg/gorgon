package service

import (
	"log/slog"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
	qbittorrentService "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	"github.com/jusoaresg/gorgon/internal/episode/model"
)

type EpisodeDownloaderInterface interface {
	DownloadEpisode(ep model.Episode, response schema.SearchResponse, score ...int) error
	DownloadEpisodeTorrent(ep model.Episode, response schema.SearchResponse, score ...int) error
	UpgradeEpisode(ep model.Episode, response schema.SearchResponse, newScore int, upgrade ...UpgradeInfo) error
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

func (d *EpisodeDownloader) DownloadEpisodeTorrent(ep model.Episode, response schema.SearchResponse, score ...int) error {
	return d.DownloadEpisode(ep, response, score...)
}

func (d *EpisodeDownloader) UpgradeEpisode(ep model.Episode, response schema.SearchResponse, newScore int, upgrade ...UpgradeInfo) error {
	logger := config.GetLogger()

	torrentService, err := qbittorrentService.NewQBittorrentService(logger)
	if err != nil {
		return err
	}

	// Add the candidate upgrade torrent into the client without deleting existing files
	if err := torrentService.AddTorrent(response.Guid); err != nil {
		logger.Error("failed to add upgrade torrent to client", slog.String("error", err.Error()))
		return err
	}

	logger.Info("added upgrade torrent to client",
		slog.String("title", response.Title),
		slog.Int("new_score", newScore),
	)

	var upgPtr *UpgradeInfo
	if len(upgrade) > 0 {
		u := upgrade[0]
		upgPtr = &u
	} else {
		upgPtr = &UpgradeInfo{NewScore: newScore}
	}

	// Persist the snatch record with the upgraded score and upgrade metadata
	if err := SnatchEpisodeWithUpgrade(&ep, response, newScore, upgPtr); err != nil {
		_ = torrentService.DeleteTorrent(response.InfoHash, true)
		return err
	}

	return nil
}
