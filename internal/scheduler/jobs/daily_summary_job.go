package jobs

import (
	"log/slog"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/telegram/service"
)

func ProcessDailySummary(db *sqlx.DB, now time.Time) error {
	logger := config.GetLogger().WithGroup("scheduler").With("job", "ProcessDailySummary")

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("Failed to load config", slog.String("error", err.Error()))
		return err
	}

	if !cfg.TelegramDailySummaryEnabled {
		logger.Debug("Daily summary is disabled in settings")
		return nil
	}

	if cfg.TelegramBotApiKey == "" || cfg.TelegramChatID == "" {
		logger.Debug("Telegram bot api key or chat ID not configured")
		return nil
	}

	summary, err := service.GetDailySummary(db, now)
	if err != nil {
		logger.Error("Failed to get daily summary", slog.String("error", err.Error()))
		return err
	}

	if len(summary.Episodes) == 0 && !cfg.TelegramNotifyEmptySummary {
		logger.Info("No episodes scheduled for today and empty summary notifications are disabled, skipping")
		return nil
	}

	tgService, err := service.NewTelegramService(logger)
	if err != nil {
		logger.Error("Failed to initialize telegram service", slog.String("error", err.Error()))
		return err
	}

	if err := tgService.SendDailySummary(summary); err != nil {
		logger.Error("Failed to send scheduled daily summary", slog.String("error", err.Error()))
		return err
	}

	logger.Info("Scheduled daily summary sent successfully", slog.Int("episodesCount", len(summary.Episodes)))
	return nil
}
