package handler

import (
	"fmt"
	"log/slog"

	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/telegram/schema"
	"github.com/jusoaresg/gorgon/external/telegram/service"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/labstack/echo/v4"
)

type TestResponse struct {
	ToastMessage string `json:"toastMessage"`
}

func TestTelegramConnection(c echo.Context) error {
	logger := config.GetLogger().WithGroup("telegramHandler").With("name", "TestTelegramConnection")
	logger.Info("Received request to test Telegram connection")

	var req schema.TestTelegramRequest
	_ = c.Bind(&req)

	apiKey := ""
	chatID := ""

	if req.TelegramBotApiKey != nil && *req.TelegramBotApiKey != "" {
		apiKey = *req.TelegramBotApiKey
	}
	if req.TelegramChatID != nil && *req.TelegramChatID != "" {
		chatID = *req.TelegramChatID
	}

	if apiKey == "" || chatID == "" {
		cfg, err := config.LoadConfig()
		if err != nil {
			logger.Error("Failed to load config file", slog.String("error", err.Error()))
			schemas.SendError(c, 500, "Failed to load config file", TestResponse{
				ToastMessage: "Failed to load config file",
			})
			return nil
		}
		if apiKey == "" {
			apiKey = cfg.TelegramBotApiKey
		}
		if chatID == "" {
			chatID = cfg.TelegramChatID
		}
	}

	if apiKey == "" || chatID == "" {
		schemas.SendError(c, 400, "Telegram Bot API Key and Chat ID are required", TestResponse{
			ToastMessage: "Bot API Key and Chat ID are required",
		})
		return nil
	}

	client := service.NewTelegramClient(logger)
	msg := schema.FormatTestNotification()
	if err := client.SendMessage(apiKey, chatID, msg); err != nil {
		logger.Error("Failed to send test telegram message", slog.String("error", err.Error()))
		schemas.SendError(c, 400, fmt.Sprintf("Failed to send test message: %s", err.Error()), TestResponse{
			ToastMessage: fmt.Sprintf("Telegram test failed: %s", err.Error()),
		})
		return nil
	}

	logger.Info("Telegram test message sent successfully")
	schemas.SendSuccess(c, "Test Telegram Connection", TestResponse{
		ToastMessage: "Telegram test notification sent successfully!",
	})
	return nil
}
