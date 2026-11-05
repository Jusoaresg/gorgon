package routes

import (
	"log/slog"

	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/internal/episode"
	episodeApi "github.com/jusoaresg/gorgon/internal/episode/api"
	"github.com/jusoaresg/gorgon/internal/filter_profile"
	filterProfileApi "github.com/jusoaresg/gorgon/internal/filter_profile/api"
	"github.com/jusoaresg/gorgon/internal/filter_settings"
	filterSettingsApi "github.com/jusoaresg/gorgon/internal/filter_settings/api"
	"github.com/jusoaresg/gorgon/internal/indexer"
	indexerApi "github.com/jusoaresg/gorgon/internal/indexer/api"
	deletionApi "github.com/jusoaresg/gorgon/internal/release_deletion/api"
	"github.com/jusoaresg/gorgon/internal/season"
	seasonApi "github.com/jusoaresg/gorgon/internal/season/api"
	"github.com/jusoaresg/gorgon/internal/show"
	showApi "github.com/jusoaresg/gorgon/internal/show/api"
	"github.com/jusoaresg/gorgon/internal/show_aliases"
	showAliasesApi "github.com/jusoaresg/gorgon/internal/show_aliases/api"
	"github.com/jusoaresg/gorgon/internal/show_settings"
	showSettingsApi "github.com/jusoaresg/gorgon/internal/show_settings/api"

	"github.com/labstack/echo/v4"
)

func SetupDatabaseRouter(v1 *echo.Group, deps *show.Dependencies, episodeDeps *episode.Dependencies, seasonDeps *season.Dependencies, indexerDeps *indexer.Dependencies, showAliasesDeps *show_aliases.Dependencies, filterProfileDeps *filter_profile.Dependencies, showSettingsDeps *show_settings.Dependencies, filterSettingsDeps *filter_settings.Dependencies) {
	logger := config.GetLogger()

	showHandler := showApi.NewHandler(deps)
	episodeHandler := episodeApi.NewHandler(episodeDeps)
	seasonHandler := seasonApi.NewHandler(seasonDeps)
	indexerHandler := indexerApi.NewHandler(indexerDeps)
	showAliasesHandler := showAliasesApi.NewHandler(showAliasesDeps)
	filterProfileHandler := filterProfileApi.NewHandler(filterProfileDeps)
	showSettingsHandler := showSettingsApi.NewHandler(showSettingsDeps)
	filterSettingsHandler := filterSettingsApi.NewHandler(filterSettingsDeps)

	deletionHandler := &deletionApi.Handler{DB: episodeDeps.DB, Logger: logger}
	v1.GET("release-deletions", deletionHandler.List)
	v1.GET("release-deletions/:id", deletionHandler.Get)
	v1.POST("release-deletions/:id/retry", deletionHandler.Retry)
	listGroup := v1.Group("database/")
	{
		showGroup := listGroup.Group("show")
		{
			showGroup.POST("", showHandler.AddShowToList)
			logger.Info("POST route added to /api/v1/database/show")

			showGroup.GET("", showHandler.ListShows)
			logger.Info("GET route added to /api/v1/database/show")

			showGroup.GET("/full", showHandler.ListFullShows)
			logger.Info("GET route added to /api/v1/database/show/full")

			showGroup.GET("/:id", showHandler.GetShow)
			logger.Info("GET route added to /api/v1/database/show/:id")

			showGroup.DELETE("/:id", showHandler.DeleteShow)
			logger.Info("DELETE route added to /api/v1/database/show/:id")

			showGroup.POST("/update-info", showHandler.UpdateShowInfo)
			logger.Info("POST route added to /api/v1/database/show/update-info")

			episodeGroup := showGroup.Group("/episode")
			{
				episodeGroup.POST("/status", episodeHandler.ChangeEpisodeStatus)
				logger.Info("POST route added to /api/v1/database/show/episode/status")

				episodeGroup.POST("/search", episodeHandler.SearchProcessEpisode)
				logger.Info("POST route added to /api/v1/database/show/episode/search")

				episodeGroup.POST("/search/all", episodeHandler.SearchProcessShowWantedEpisodes)
				logger.Info("POST route added to /api/v1/database/show/episode/search/all")

				episodeGroup.POST("/search/season/:id", episodeHandler.SearchProcessSeasonEpisodes)
				logger.Info("POST route added to /api/v1/database/show/episode/search/season/:id")

				episodeGroup.GET("/:id", episodeHandler.GetShowEpisodes)
				logger.Info("GET route added to /api/v1/database/show/episode/:id")

				episodeGroup.DELETE("/:id", episodeHandler.DeleteDownloadedEpisode)
				logger.Info("DELETE route added to /api/v1/database/show/episode/:id")
			}

			aliasGroup := showGroup.Group("/aliases")
			{
				aliasGroup.GET("/:id", showAliasesHandler.GetShowAliases)
			}

			showGroup.POST("/:id/alias", showAliasesHandler.AddShowAlias)
			logger.Info("POST route added to /api/v1/database/show/:id/alias")

			showGroup.DELETE("/:id/alias/:aliasId", showAliasesHandler.DeleteShowAlias)
			logger.Info("DELETE route added to /api/v1/database/show/:id/alias/:aliasId")

			seasonsGroup := showGroup.Group("/season")
			{
				seasonsGroup.GET("/:id", seasonHandler.GetShowSeasons)
				logger.Info("GET route added to /api/v1/database/show/season/:id")
			}
		}

		filterProfileGroup := listGroup.Group("filter-profile")
		{
			filterProfileGroup.POST("", filterProfileHandler.CreateFilterProfile)
			logger.Info("POST route added to /api/v1/database/filter-profile")

			filterProfileGroup.GET("", filterProfileHandler.ListFilterProfiles)
			logger.Info("GET route added to /api/v1/database/filter-profile")

			filterProfileGroup.GET("/:id", filterProfileHandler.GetFilterProfile)
			logger.Info("GET route added to /api/v1/database/filter-profile/:id")

			filterProfileGroup.PUT("/:id", filterProfileHandler.UpdateFilterProfile)
			logger.Info("PUT route added to /api/v1/database/filter-profile/:id")

			filterProfileGroup.DELETE("/:id", filterProfileHandler.DeleteFilterProfile)
			logger.Info("DELETE route added to /api/v1/database/filter-profile/:id")
		}

		showSettingsGroup := listGroup.Group("show-settings")
		{
			showSettingsGroup.GET("/:id", showSettingsHandler.GetShowSettings)
			logger.Info("GET route added to /api/v1/database/show-settings/:id")

			showSettingsGroup.PUT("/:id", showSettingsHandler.UpdateShowSettings)
			logger.Info("PUT route added to /api/v1/database/show-settings/:id")
		}

		filterSettingsGroup := listGroup.Group("filter-settings")
		{
			filterSettingsGroup.GET("", filterSettingsHandler.GetFilterSettings)
			logger.Info("GET route added to /api/v1/database/filter-settings")

			filterSettingsGroup.PATCH("", filterSettingsHandler.UpdateFilterSettings)
			logger.Info("PATCH route added to /api/v1/database/filter-settings")
		}

		indexerGroup := listGroup.Group("indexer")
		{
			indexerGroup.GET(":id", indexerHandler.GetIndexer)
			logger.Info("GET route added to /api/v1/database/indexer/:id")
			indexerGroup.GET("", indexerHandler.ListIndexers)
			logger.Info("GET route added to /api/v1/database/indexer")

			indexerGroup.POST("", indexerHandler.AddIndexer)
			logger.Info("POST route added to /api/v1/database/indexer")

			indexerGroup.DELETE("", indexerHandler.DeleteIndexer)
			logger.Info("DELETE route added to /api/v1/database/indexer")
		}
	}
	logger.Info("Database routes added successfully", slog.String("endpoint", "/api/v1/database"))
}
