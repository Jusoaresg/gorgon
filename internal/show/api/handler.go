package api

import (
	"log/slog"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	qbittorrentService "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	tvMazeService "github.com/jusoaresg/gorgon/external/tvmaze/service"
	"github.com/jusoaresg/gorgon/internal/show"
	"github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/internal/show/service"
	showAliasRepository "github.com/jusoaresg/gorgon/internal/show_aliases/repository"
	"github.com/jusoaresg/gorgon/pkg/schemas"
)

type Handler struct {
	ShowRepo           repository.ShowRepositoryInterface
	ShowAliasRepo      showAliasRepository.ShowAliasesRepositoryInterface
	AggregatorService  service.ShowAggregatorService
	TvMazeService      tvMazeService.TvMazeSearchService
	ShowManager        tvMazeService.ShowManager
	ShowManagerService *service.ShowManagerService
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

func NewHandler(deps *show.Dependencies) *Handler {
	return &Handler{
		ShowRepo:           deps.ShowRepo,
		ShowAliasRepo:      deps.ShowAliasesRepo,
		AggregatorService:  deps.AggregatorService,
		TvMazeService:      deps.TvMazeService,
		ShowManager:        deps.ShowManager,
		ShowManagerService: deps.ShowManagerService,
		DB:                 deps.DB,
		Logger:             deps.Logger,
	}
}
