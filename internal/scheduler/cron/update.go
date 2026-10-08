package cron

import (
	"context"
	"log/slog"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
)

const DefaultShowsUpdateInterval = 6 * time.Hour

func StartDailyUpdate(ctx context.Context, callback func()) {
	StartShowsUpdate(ctx, nil, DefaultShowsUpdateInterval, callback)
}

func StartShowsUpdate(ctx context.Context, db *sqlx.DB, interval time.Duration, callback func()) {
	if interval <= 0 {
		interval = DefaultShowsUpdateInterval
	}
	logger := config.GetLogger().WithGroup("scheduler").With("name", "StartShowsUpdate")

	lastUpdate, err := GetLastShowsUpdateTime(db)
	if err != nil {
		logger.Warn("failed to get last shows update time, proceeding with default behavior", slog.String("error", err.Error()))
	}

	var initialDelay time.Duration

	if lastUpdate.IsZero() || time.Since(lastUpdate) >= interval {
		logger.Info("running initial show update on startup")
		callback()
		logger.Info("shows update completed")
		initialDelay = interval
	} else {
		elapsed := time.Since(lastUpdate)
		initialDelay = interval - elapsed
		logger.Info("skipping startup show update, recent update found",
			slog.Duration("time_since_last_update", elapsed),
			slog.Duration("next_update_in", initialDelay),
		)
	}

	timer := time.NewTimer(initialDelay)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("shows update stopped")
			return
		case <-timer.C:
			logger.Info("starting to updating shows")
			callback()
			logger.Info("shows update completed")
			timer.Reset(interval)
		}
	}
}
