package routes

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/docs"
	"github.com/jusoaresg/gorgon/internal/app"
	downloadsRouter "github.com/jusoaresg/gorgon/internal/downloads/web"
	episodeRouter "github.com/jusoaresg/gorgon/internal/episode/web"
	showRouter "github.com/jusoaresg/gorgon/internal/show/web"
	"github.com/jusoaresg/gorgon/views"
	"github.com/labstack/echo/v4"
)

const basePath = "/api/v1/"

func InitializeRoutes(e *echo.Echo, deps *app.Dependencies) {
	logger := config.GetLogger().WithGroup("routes").With("name", "initializeRoutes")

	e.Renderer = views.NewTemplate()

	staticFS, err := fs.Sub(views.FrontStaticFS, "static")
	if err != nil {
		panic(fmt.Errorf("error loading static files"))
	}
	e.StaticFS("/static", staticFS)

	docs.SwaggerInfo.BasePath = basePath
	logger.Info("Initializing routes", slog.String("basePath", basePath))

	showRouter.RegisterShowRoutes(e, deps.Show)
	logger.Debug("Show web routes initialized successfully")

	episodeRouter.RegisterEpisodeRoutes(e, deps.Episode)
	logger.Debug("Show web routes initialized successfully")

	downloadsRouter.RegisterDownloadsRoutes(e, deps.Downloads)
	logger.Debug("Downloads web routes initialized successfully")

	v1 := e.Group(basePath)

	SetupTvMazeRouter(v1)
	logger.Debug("TvMaze route initialized successfully")

	SetupDatabaseRouter(v1, deps.Show, deps.Episode, deps.Season, deps.Indexer, deps.ShowAliases, deps.FilterProfile, deps.ShowSettings, deps.FilterSettings)
	logger.Debug("Database route initialized successfully")

	SetupProwlarrRouter(v1)
	logger.Debug("Prowlarr route initialized successfully")

	SetupQbittorrentRouter(v1)
	logger.Debug("QBittorrent route initialized successfully")

	SetupWebsocketRouter(v1)
	logger.Debug("WebSocket route initialized successfully")

	SetupAppConfigRouter(v1, deps.AppConfig)
	logger.Debug("App Config route initialized successfully")

	SetupTelegramRouter(v1)
	logger.Debug("Telegram route initialized successfully")

	e.GET("/swagger.json", func(c echo.Context) error {
		return c.Blob(http.StatusOK, "application/json", docs.SwaggerJSON)
	})

	v1.GET("docs", func(c echo.Context) error {
		return c.Render(http.StatusOK, "docs.html", nil)
	})

	logger.Info("Routes initialized", slog.String("basePath", basePath))
}
