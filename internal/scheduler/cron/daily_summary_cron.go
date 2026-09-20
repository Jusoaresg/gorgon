package cron

import (
	"log/slog"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/internal/scheduler/jobs"
)

func StartDailySummaryCron(db *sqlx.DB) {
	logger := config.GetLogger().WithGroup("cron").With("name", "DailySummaryCron")

	go func() {
		var lastSentDate string

		for {
			now := time.Now().In(config.GetAppLocation())
			currentDate := now.Format("2006-01-02")
			currentTime := now.Format("15:04")

			cfg, err := config.LoadConfig()
			if err == nil && cfg.TelegramDailySummaryEnabled {
				targetTime := cfg.TelegramDailySummaryTime
				if targetTime == "" {
					targetTime = "08:00"
				}

				if currentTime == targetTime && lastSentDate != currentDate {
					logger.Info("Triggering scheduled daily summary", slog.String("time", currentTime), slog.String("date", currentDate))
					if err := jobs.ProcessDailySummary(db, now); err != nil {
						logger.Error("Error sending scheduled daily summary", slog.String("error", err.Error()))
					} else {
						lastSentDate = currentDate
					}
				}
			}

			time.Sleep(30 * time.Second)
		}
	}()
}
