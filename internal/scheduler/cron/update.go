package cron

import (
	"context"
	"time"

	"github.com/jusoaresg/gorgon/config"
)

func StartDailyUpdate(ctx context.Context, callback func()) {
	logger := config.GetLogger().WithGroup("scheduler").With("name", "StartDailyUpdate")
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	logger.Info("starting to updating shows")
	callback()
	logger.Info("shows update completed")

	for {
		select {
		case <-ctx.Done():
			logger.Info("daily update stopped")
			return
		case <-ticker.C:
			logger.Info("starting to updating shows")
			callback()
			logger.Info("shows update completed")
		}
	}
}
