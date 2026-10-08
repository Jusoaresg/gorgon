package scheduler

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/tvmaze/service"
	showHandler "github.com/jusoaresg/gorgon/internal/show/api"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	showManager "github.com/jusoaresg/gorgon/internal/show/service"
	"github.com/jusoaresg/gorgon/pkg/services"
)

func UpdateAllShows() {
	UpdateAllShowsWithDB(config.GetSQLite())
}

func UpdateAllShowsWithDB(db *sqlx.DB) {
	if db == nil {
		return
	}
	logger := config.GetLogger().WithGroup("scheduler").With("name", "UpdateAllShows")

	showRepo := showRepository.NewShowRepository(db)
	shows, err := showRepo.List()
	if err != nil {
		logger.Error("failed to list shows from repository", slog.String("error", err.Error()))
		return
	}

	tvMazeService := service.NewTvMazeSearchService(logger)
	showManagerService := showManager.NewShowManagerService(logger, db)
	apiService := services.NewAPIService("http://api.tvmaze.com", logger)

	var updates map[string]int64

	url := "/updates/shows?since=week"
	if err := apiService.Get(url, &updates); err != nil {
		logger.Error("failed to get updated shows from tvmaze", slog.String("error", err.Error()))
		return
	}

	updatedCount := 0
	for _, showOld := range shows {
		updatedAt, ok := updates[strconv.FormatInt(showOld.TvMazeID, 10)]
		if !ok {
			continue
		}

		if int64(showOld.Updated) < updatedAt {

			logger.Info(
				"updating show",
				slog.Int64("show_id", showOld.ID),
				slog.Int64("tv_maze_id", showOld.TvMazeID),
				slog.String("title", showOld.Name),
			)

			_, err := showHandler.UpdateSingleShowInfo(
				showRepo,
				tvMazeService,
				showManagerService,
				logger,
				showOld.ID,
			)
			if err != nil {
				continue
			}

			updatedCount++
			time.Sleep(15 * time.Second)
		}
	}

	logger.Info("finished updating shows", slog.Int("total_updated", updatedCount))
}
