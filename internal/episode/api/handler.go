package api

import (
	"context"
	"github.com/jusoaresg/gorgon/internal/release_deletion"
	"log/slog"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/internal/episode"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	"github.com/jusoaresg/gorgon/internal/episode/service"
	epContentRepository "github.com/jusoaresg/gorgon/internal/episode_content/repository"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
)

type DeletionCoordinator interface {
	Delete(context.Context, release_deletion.Request) (release_deletion.Operation, error)
}

type Handler struct {
	DeletionService    DeletionCoordinator
	EpisodeRepo        episodeRepository.EpisodeRepositoryInterface
	EpisodeContentRepo epContentRepository.EpisodeContentRepository
	EpisodeTorrentRepo episodeTorrentRepository.EpisodeTorrentRepositoryInterface
	ShowRepo           showRepository.ShowRepositoryInterface
	EpisodeSearchSvc   *service.EpisodeSearchService
	DB                 *sqlx.DB
	Logger             *slog.Logger
}

func NewHandler(deps *episode.Dependencies) *Handler {
	return &Handler{
		EpisodeRepo:        deps.EpisodeRepo,
		EpisodeContentRepo: deps.EpisodeContentRepo,
		EpisodeTorrentRepo: deps.EpisodeTorrentRepo,
		ShowRepo:           deps.ShowRepo,
		EpisodeSearchSvc:   deps.EpisodeSearchSvc,
		DB:                 deps.DB,
		Logger:             deps.Logger,
	}
}
