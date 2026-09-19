package service

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/telegram/schema"
)

func StartTelegramBotListener(db *sqlx.DB) {
	logger := config.GetLogger().WithGroup("telegramBot").With("name", "BotListener")

	go func() {
		logger.Info("Starting Telegram bot listener")
		var offset int64 = 0

		client := NewTelegramClient(logger)

		for {
			cfg, err := config.LoadConfig()
			if err != nil || cfg.TelegramBotApiKey == "" {
				time.Sleep(10 * time.Second)
				continue
			}

			token := cfg.TelegramBotApiKey

			endpoint := fmt.Sprintf("bot%s/getUpdates?offset=%d&timeout=20", token, offset)
			var resp schema.GetUpdatesResponse
			if err := client.ApiService.Get(endpoint, &resp); err != nil {
				logger.Debug("getUpdates poll cycle completed or timed out", slog.String("error", err.Error()))
				time.Sleep(3 * time.Second)
				continue
			}

			if !resp.Ok {
				logger.Debug("getUpdates returned not ok", slog.String("description", resp.Description))
				time.Sleep(5 * time.Second)
				continue
			}

			for _, u := range resp.Result {
				if u.UpdateID >= offset {
					offset = u.UpdateID + 1
				}

				var msg *schema.TelegramUpdateMessage
				if u.Message != nil {
					msg = u.Message
				} else if u.ChannelPost != nil {
					msg = u.ChannelPost
				}

				if msg == nil || msg.Text == "" {
					continue
				}

				text := strings.TrimSpace(msg.Text)
				// Handle bot username suffixes like /summary@MyGorgonBot
				if idx := strings.Index(text, "@"); idx != -1 && strings.HasPrefix(text, "/") {
					parts := strings.Fields(text)
					if len(parts) > 0 {
						cmdParts := strings.Split(parts[0], "@")
						parts[0] = cmdParts[0]
						text = strings.Join(parts, " ")
					}
				}

				chatID := strconv.FormatInt(msg.Chat.ID, 10)
				cmd := strings.ToLower(text)

				switch {
				case strings.HasPrefix(cmd, "/summary") || strings.HasPrefix(cmd, "/today"):
					logger.Info("Received summary command", slog.String("chatID", chatID))
					summaryText := GenerateDailySummaryText(db, time.Now())
					if err := client.SendMessage(token, chatID, summaryText); err != nil {
						logger.Error("Failed to send summary reply", slog.String("error", err.Error()))
					}

				case strings.HasPrefix(cmd, "/ping"):
					logger.Info("Received ping command", slog.String("chatID", chatID))
					_ = client.SendMessage(token, chatID, "🏓 <b>Pong!</b> Gorgon is running smoothly.")

				case strings.HasPrefix(cmd, "/start") || strings.HasPrefix(cmd, "/help"):
					logger.Info("Received start/help command", slog.String("chatID", chatID))
					welcome := "👋 <b>Welcome to Gorgon Bot!</b>\n\n" +
						"Here are the available commands:\n" +
						"• <code>/summary</code> — View today's scheduled releases\n" +
						"• <code>/ping</code> — Check bot connection"
					_ = client.SendMessage(token, chatID, welcome)
				}
			}
		}
	}()
}
