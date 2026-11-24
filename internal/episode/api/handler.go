package api

import (
	"log/slog"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	qbittorrentService "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	"github.com/jusoaresg/gorgon/internal/episode"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	"github.com/jusoaresg/gorgon/internal/episode/service"
	epContentRepository "github.com/jusoaresg/gorgon/internal/episode_content/repository"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/pkg/schemas"
)

type Handler struct {
	EpisodeRepo        episodeRepository.EpisodeRepositoryInterface
	EpisodeContentRepo epContentRepository.EpisodeContentRepository
	EpisodeTorrentRepo episodeTorrentRepository.EpisodeTorrentRepositoryInterface
	ShowRepo           showRepository.ShowRepositoryInterface
	EpisodeSearchSvc   *service.EpisodeSearchService
	DB                 *sqlx.DB
	Logger             *slog.Logger

	// Optional overrides for testing.
	NewTorrentClient func(*slog.Logger) (qbittorrentService.TorrentDeleter, error)
	LoadConfig       func() (*schemas.ConfigFile, error)
}

func (h *Handler) newTorrentClient() (qbittorrentService.TorrentDeleter, error) {
	if h.NewTorrentClient != nil {
		return h.NewTorrentClient(h.Logger)
	}
	return qbittorrentService.NewQBittorrentService(h.Logger)
}

func (h *Handler) loadConfig() (*schemas.ConfigFile, error) {
	if h.LoadConfig != nil {
		return h.LoadConfig()
	}
	return config.LoadConfig()
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
