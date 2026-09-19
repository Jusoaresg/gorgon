package routes

import (
	"log/slog"

	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/telegram/handler"
	"github.com/labstack/echo/v4"
)

func SetupTelegramRouter(r *echo.Group) {
	logger := config.GetLogger()

	telegramRouter := r.Group("telegram/")
	{
		telegramRouter.POST("test", handler.TestTelegramConnection)
		logger.Info("POST route added to /api/v1/telegram/test")
		telegramRouter.POST("detect-chat-id", handler.DetectTelegramChatID)
		logger.Info("POST route added to /api/v1/telegram/detect-chat-id")
	}
	logger.Info("Telegram routes added successfully", slog.String("endpoint", "/api/v1/telegram"))
}
