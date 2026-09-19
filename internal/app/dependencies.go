package app

import (
	"log/slog"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	appConfig "github.com/jusoaresg/gorgon/internal/config"
	"github.com/jusoaresg/gorgon/internal/downloads"
	"github.com/jusoaresg/gorgon/internal/episode"
	"github.com/jusoaresg/gorgon/internal/filter_profile"
	"github.com/jusoaresg/gorgon/internal/filter_settings"
	"github.com/jusoaresg/gorgon/internal/indexer"
	"github.com/jusoaresg/gorgon/internal/season"
	"github.com/jusoaresg/gorgon/internal/show"
	"github.com/jusoaresg/gorgon/internal/show_aliases"
	"github.com/jusoaresg/gorgon/internal/show_settings"
)

type Dependencies struct {
	Show           *show.Dependencies
	Episode        *episode.Dependencies
	Season         *season.Dependencies
	Indexer        *indexer.Dependencies
	Downloads      *downloads.Dependencies
	AppConfig      *appConfig.Dependencies
	ShowAliases    *show_aliases.Dependencies
	FilterProfile  *filter_profile.Dependencies
	ShowSettings   *show_settings.Dependencies
	FilterSettings *filter_settings.Dependencies
	Logger         *slog.Logger
	DB             *sqlx.DB
}

func NewDependencies() *Dependencies {
	logger := config.GetLogger()
	db := config.GetSQLite()

	return &Dependencies{
		Show:           show.NewDependencies(db, logger),
		Episode:        episode.NewDependencies(db, logger),
		Season:         season.NewDependencies(db, logger),
		Indexer:        indexer.NewDependencies(db, logger),
		Downloads:      downloads.NewDependencies(db, logger),
		AppConfig:      appConfig.NewDependencies(config.GetConfigService(), logger),
		ShowAliases:    show_aliases.NewDependencies(db, logger),
		FilterProfile:  filter_profile.NewDependencies(db, logger),
		ShowSettings:   show_settings.NewDependencies(db, logger),
		FilterSettings: filter_settings.NewDependencies(db, logger),
		Logger:         logger,
		DB:             db,
	}
}
