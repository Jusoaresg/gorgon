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

type DetectChatIDResponse struct {
	ChatID       string `json:"chatID"`
	ToastMessage string `json:"toastMessage"`
}

func DetectTelegramChatID(c echo.Context) error {
	logger := config.GetLogger().WithGroup("telegramHandler").With("name", "DetectTelegramChatID")
	logger.Info("Received request to auto-detect Telegram Chat ID")

	var req schema.TestTelegramRequest
	_ = c.Bind(&req)

	apiKey := ""
	if req.TelegramBotApiKey != nil && *req.TelegramBotApiKey != "" {
		apiKey = *req.TelegramBotApiKey
	}

	if apiKey == "" {
		cfg, err := config.LoadConfig()
		if err != nil {
			logger.Error("Failed to load config file", slog.String("error", err.Error()))
			schemas.SendError(c, 500, "Failed to load config file", DetectChatIDResponse{
				ToastMessage: "Failed to load config file",
			})
			return nil
		}
		apiKey = cfg.TelegramBotApiKey
	}

	if apiKey == "" {
		schemas.SendError(c, 400, "Bot API Token is required to detect Chat ID", DetectChatIDResponse{
			ToastMessage: "Please enter your Bot API Token first",
		})
		return nil
	}

	client := service.NewTelegramClient(logger)
	chatID, err := client.GetLatestChatID(apiKey)
	if err != nil {
		logger.Error("Failed to auto-detect Telegram chat ID", slog.String("error", err.Error()))
		schemas.SendError(c, 400, fmt.Sprintf("Failed to detect Chat ID: %s", err.Error()), DetectChatIDResponse{
			ToastMessage: err.Error(),
		})
		return nil
	}

	logger.Info("Telegram chat ID detected successfully", slog.String("chatID", chatID))
	schemas.SendSuccess(c, "Detect Telegram Chat ID", DetectChatIDResponse{
		ChatID:       chatID,
		ToastMessage: fmt.Sprintf("Chat ID detected: %s", chatID),
	})
	return nil
}
