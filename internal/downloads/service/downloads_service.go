package service

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
)

const gorgonCategory = "gorgon"

var finishedStates = map[string]bool{
	"uploading":       true,
	"stalledUP":       true,
	"queuedUP":        true,
	"pausedUP":        true,
	"stoppedUP":       true,
	"forcedUP":        true,
	"forcedUploading": true,
	"checkingUP":      true,
	"error":           true,
	"missingFiles":    true,
	"unknown":         true,
}

type EpisodeInfo struct {
	EpisodeID int64  `db:"episode_id"`
	ShowID    int64  `db:"show_id"`
	Name      string `db:"name"`
	Season    int    `db:"season"`
	Number    int    `db:"number"`
	Tracking  string `db:"tracking"`
	ShowName  string `db:"show_name"`
	ShowImage string `db:"show_image"`
	Hash      string `db:"hash"`
	Title     string `db:"title"`
	Indexer   string `db:"indexer"`
	InfoUrl   string `db:"info_url"`
}

type DownloadItem struct {
	Torrent schema.CheckTorrentResponse
	Episode *EpisodeInfo
}

type DownloadsSummary struct {
	TotalDlSpeed int
	TotalUpSpeed int
	ActiveCount  int
}

type DownloadsService struct {
	db     *sqlx.DB
	logger *slog.Logger
}

func NewDownloadsService(db *sqlx.DB, logger *slog.Logger) *DownloadsService {
	return &DownloadsService{
		db:     db,
		logger: logger,
	}
}

func (s *DownloadsService) BuildDownloads(torrents []schema.CheckTorrentResponse) ([]DownloadItem, error) {
	hashes := make([]string, 0, len(torrents))
	for _, torrent := range torrents {
		if torrent.Category == gorgonCategory && torrent.Hash != "" {
			hashes = append(hashes, strings.ToLower(torrent.Hash))
		}
	}

	episodesByHash, err := s.episodesByHashes(hashes)
	if err != nil {
		return nil, fmt.Errorf("failed to load episodes by torrent hash: %w", err)
	}

	items := make([]DownloadItem, 0, len(torrents))
	for _, torrent := range torrents {
		if torrent.Category != gorgonCategory {
			continue
		}

		var episode *EpisodeInfo
		if info, ok := episodesByHash[strings.ToLower(torrent.Hash)]; ok {
			ep := info
			episode = &ep
		}

		if finishedStates[torrent.State] {
			if episode == nil || episode.Tracking != episodeModel.TrackingSnatched {
				continue
			}
		}

		items = append(items, DownloadItem{
			Torrent: torrent,
			Episode: episode,
		})
	}

	sortDownloads(items)
	return items, nil
}

func (s *DownloadsService) ComputeSummary(items []DownloadItem) DownloadsSummary {
	var summary DownloadsSummary
	for _, item := range items {
		summary.TotalDlSpeed += item.Torrent.DlSpeed
		summary.TotalUpSpeed += item.Torrent.UpSpeed
		if !finishedStates[item.Torrent.State] {
			summary.ActiveCount++
		}
	}
	return summary
}

func sortDownloads(items []DownloadItem) {
	sort.SliceStable(items, func(i, j int) bool {
		iActive := !finishedStates[items[i].Torrent.State]
		jActive := !finishedStates[items[j].Torrent.State]
		if iActive != jActive {
			return iActive
		}
		return items[i].Torrent.AddedOn < items[j].Torrent.AddedOn
	})
}

func (s *DownloadsService) episodesByHashes(hashes []string) (map[string]EpisodeInfo, error) {
	if len(hashes) == 0 {
		return map[string]EpisodeInfo{}, nil
	}

	query, args, err := sqlx.In(`
		SELECT e.id AS episode_id, e.show_id, e.name, e.season, e.number, e.tracking,
		       s.name AS show_name, s.image_medium AS show_image,
		       et.hash AS hash, et.title AS title, et.indexer AS indexer, et.info_url AS info_url
		FROM episodes e
		JOIN shows s ON e.show_id = s.id
		JOIN episode_torrents et ON et.episode_id = e.id
		WHERE LOWER(et.hash) IN (?)
	`, hashes)
	if err != nil {
		return nil, err
	}

	query = s.db.Rebind(query)
	var rows []EpisodeInfo
	if err := s.db.Select(&rows, query, args...); err != nil {
		return nil, err
	}

	episodesByHash := make(map[string]EpisodeInfo, len(rows))
	for _, row := range rows {
		episodesByHash[strings.ToLower(row.Hash)] = row
	}

	return episodesByHash, nil
}
