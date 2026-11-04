package web

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/internal/downloads"
	"github.com/jusoaresg/gorgon/internal/downloads/service"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	"github.com/jusoaresg/gorgon/internal/release_deletion"
)

type DeletionCoordinator interface {
	Delete(context.Context, release_deletion.Request) (release_deletion.Operation, error)
}

type Handler struct {
	DeletionService    DeletionCoordinator
	Service            *service.DownloadsService
	EpisodeRepo        episodeRepository.EpisodeRepositoryInterface
	EpisodeTorrentRepo episodeTorrentRepository.EpisodeTorrentRepositoryInterface
	DB                 *sqlx.DB
}

func NewHandler(deps *downloads.Dependencies) *Handler {
	return &Handler{
		Service:            deps.Service,
		EpisodeRepo:        deps.EpisodeRepo,
		EpisodeTorrentRepo: deps.EpisodeTorrentRepo,
		DB:                 deps.DB,
	}
}
