package cron

import (
	"context"
	"time"

	"github.com/jusoaresg/gorgon/config"
)

func StartVerifyEpisodeWasDeleted(ctx context.Context, callback func()) {
	logger := config.GetLogger().WithGroup("cron").With("name", "StartVerifyEpisodeWasDeleted")
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("verification of deleted episodes stopped")
			return
		case <-ticker.C:
			logger.Info("Starting to verifying if any episode has been deleted")
			callback()
			logger.Info("Verification of deleted episodes completed")
		}
	}
}
