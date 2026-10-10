package model

import (
	"time"

	prowlarrSchema "github.com/jusoaresg/gorgon/external/prowlarr/schema"
	qbittorrentSchema "github.com/jusoaresg/gorgon/external/qbittorrent/schema"
)

type EpisodeTorrent struct {
	ID        int64 `db:"id"`
	EpisodeId int64 `db:"episode_id"`

	Hash        string `db:"hash"`
	Title       string `db:"title"`
	Indexer     string `db:"indexer"`
	InfoUrl     string `db:"info_url"`
	PublishDate string `db:"publish_date"`
	Score       int    `db:"score"`
	CreatedAt   int64  `db:"created_at"`
}

func FromSearchResponse(episodeID int64, r prowlarrSchema.SearchResponse, score ...int) EpisodeTorrent {
	s := 0
	if len(score) > 0 {
		s = score[0]
	}
	return EpisodeTorrent{
		EpisodeId:   episodeID,
		Hash:        r.InfoHash,
		Title:       r.Title,
		Indexer:     r.Indexer,
		InfoUrl:     r.InfoUrl,
		PublishDate: r.PublishDate,
		Score:       s,
		CreatedAt:   time.Now().Unix(),
	}
}

func FromAddTorrentRequest(episodeID int64, r qbittorrentSchema.AddEpisodeTorrentRequest, score ...int) EpisodeTorrent {
	s := 0
	if len(score) > 0 {
		s = score[0]
	}
	return EpisodeTorrent{
		EpisodeId:   episodeID,
		Hash:        r.InfoHash,
		Title:       r.Title,
		Indexer:     r.Indexer,
		InfoUrl:     r.InfoUrl,
		PublishDate: r.PublishDate,
		Score:       s,
		CreatedAt:   time.Now().Unix(),
	}
}
