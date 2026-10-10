package scheduler

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/tvmaze/service"
	"github.com/jusoaresg/gorgon/internal/scheduler/cron"
	showHandler "github.com/jusoaresg/gorgon/internal/show/api"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	showManager "github.com/jusoaresg/gorgon/internal/show/service"
	"github.com/jusoaresg/gorgon/pkg/services"
)

func UpdateAllShows() {
	UpdateAllShowsWithDB(config.GetSQLite())
}

func UpdateAllShowsWithDB(db *sqlx.DB) {
	UpdateAllShowsWithContext(context.Background(), db)
}

func UpdateAllShowsWithContext(ctx context.Context, db *sqlx.DB) {
	if db == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
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
	apiService := services.NewAPIService("https://api.tvmaze.com", logger)

	var updates map[string]int64

	url := "/updates/shows?since=week"
	if err := apiService.GetContext(ctx, url, &updates); err != nil {
		logger.Error("failed to get updated shows from tvmaze", slog.String("error", err.Error()))
		return
	}

	updatedCount := 0
	for _, showOld := range shows {
		if ctx.Err() != nil {
			logger.Info("show updates stopped due to context cancellation")
			return
		}

		updatedAt, ok := updates[strconv.FormatInt(showOld.TvMazeID, 10)]
		shouldUpdate := false
		if ok && int64(showOld.Updated) < updatedAt {
			shouldUpdate = true
		} else if showOld.Updated == 0 {
			shouldUpdate = true
		}

		if shouldUpdate {
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
				logger.Error("failed to update show in batch, continuing to next show",
					slog.Int64("show_id", showOld.ID),
					slog.Int64("tv_maze_id", showOld.TvMazeID),
					slog.String("title", showOld.Name),
					slog.String("error", err.Error()),
				)
				time.Sleep(300 * time.Millisecond)
				continue
			}

			updatedCount++

			time.Sleep(300 * time.Millisecond)
		}
	}

	logger.Info("finished updating shows", slog.Int("total_updated", updatedCount))
	_ = cron.SetLastShowsUpdateTime(db, time.Now())
}
