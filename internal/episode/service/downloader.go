package service

import (
	"context"
	"errors"
	"fmt"
	qs "github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	"github.com/jusoaresg/gorgon/internal/release_deletion"
	"github.com/jusoaresg/gorgon/internal/release_guard"
	"strings"

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
	db            *sqlx.DB
	TorrentClient AcquisitionClient
}

// AcquisitionClient exposes both ownership-aware acquisition and reliable cleanup.
type AcquisitionClient interface {
	release_deletion.Client
	AddTorrentIfAbsent(string) (bool, error)
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
	value := 0
	if len(score) > 0 {
		value = score[0]
	}
	return d.acquire(ep, response, value, nil)
}

func (d *EpisodeDownloader) acquire(ep model.Episode, response schema.SearchResponse, score int, upgrade *UpgradeInfo) error {
	db := d.getDB()
	unlock := release_guard.Lock(db)
	defer unlock()
	hash := strings.ToLower(strings.TrimSpace(response.InfoHash))
	if hash == "" {
		return fmt.Errorf("candidate torrent hash is required to establish ownership")
	}
	if err := validateSnatch(db, &ep, hash); err != nil {
		return err
	}
	var associated int
	if err := db.Get(&associated, "SELECT count(*) FROM episode_torrents WHERE lower(trim(hash))=?", hash); err != nil {
		return err
	}
	if associated > 0 {
		return fmt.Errorf("candidate torrent already belongs to an episode")
	}
	logger := config.GetLogger()
	client := d.TorrentClient
	if client == nil {
		var err error
		client, err = qbittorrentService.NewQBittorrentService(logger)
		if err != nil {
			return err
		}
	}
	var existing []qs.CheckTorrentResponse
	if err := client.CheckTorrentsWithHash("", hash, &existing); err != nil {
		return err
	}
	if len(existing) > 0 {
		return fmt.Errorf("candidate torrent already exists; acquisition has no cleanup ownership")
	}
	owned, err := client.AddTorrentIfAbsent(response.Guid)
	if err != nil {
		return err
	}
	if !owned {
		return fmt.Errorf("candidate torrent already exists; acquisition has no cleanup ownership")
	}
	if err = snatchEpisodeLocked(db, &ep, response, score, upgrade); err != nil {
		cfg, cfgErr := config.LoadConfig()
		if cfgErr != nil {
			return errors.Join(err, cfgErr, fmt.Errorf("new torrent requires cleanup"))
		}
		deletion := release_deletion.NewWithOptions(db, logger, release_deletion.Options{Client: client, DownloadRoot: cfg.QBittorrentDownloadFolder, ShowsRoot: cfg.ShowsFolder})
		op, cleanupErr := deletion.DeleteWhileGuarded(context.Background(), release_deletion.Request{Hash: hash, Intention: "skipped"})
		if cleanupErr != nil {
			return errors.Join(err, fmt.Errorf("acquisition cleanup incomplete (operation %s): %w", op.ID, cleanupErr))
		}
		return err
	}
	return nil
}

func (d *EpisodeDownloader) DownloadEpisodeTorrent(ep model.Episode, response schema.SearchResponse, score ...int) error {
	return d.DownloadEpisode(ep, response, score...)
}

func (d *EpisodeDownloader) UpgradeEpisode(ep model.Episode, response schema.SearchResponse, newScore int, upgrade ...UpgradeInfo) error {
	info := &UpgradeInfo{NewScore: newScore}
	if len(upgrade) > 0 {
		copy := upgrade[0]
		info = &copy
	}
	return d.acquire(ep, response, newScore, info)
}
