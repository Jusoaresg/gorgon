package service

import (
	"log/slog"

	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
	qbittorrentService "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	"github.com/jusoaresg/gorgon/internal/episode/model"
)

type EpisodeDownloaderInterface interface {
	DownloadEpisode(ep model.Episode, response schema.SearchResponse) error
	DownloadEpisodeTorrent(ep model.Episode, response schema.SearchResponse) error
}

type EpisodeDownloader struct{}

func (d *EpisodeDownloader) DownloadEpisode(ep model.Episode, response schema.SearchResponse) error {
	return d.DownloadEpisodeTorrent(ep, response)
}

func (d *EpisodeDownloader) DownloadEpisodeTorrent(ep model.Episode, response schema.SearchResponse) error {
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

	if err := SnatchEpisode(&ep, response); err != nil {
		if err := torrentService.DeleteTorrent(response.InfoHash, true); err != nil {
			return err
		}
		return err
	}

	return nil
}

