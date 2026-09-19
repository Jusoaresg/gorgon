package service

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/telegram/schema"
	apiService "github.com/jusoaresg/gorgon/pkg/services"
)

var ErrTelegramNotConfigured = errors.New("telegram bot api key or chat id not configured")

type TelegramService struct {
	ApiService *apiService.APIService
	Logger     *slog.Logger
	ApiKey     string
	ChatID     string
}

func NewTelegramService(logger *slog.Logger) (*TelegramService, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}

	if cfg.TelegramBotApiKey == "" || cfg.TelegramChatID == "" {
		return nil, ErrTelegramNotConfigured
	}

	return &TelegramService{
		ApiService: apiService.NewAPIService("https://api.telegram.org/", logger),
		Logger:     logger.WithGroup("telegramService"),
		ApiKey:     cfg.TelegramBotApiKey,
		ChatID:     cfg.TelegramChatID,
	}, nil
}

// NewTelegramClient creates a TelegramService instance for direct messaging (e.g. testing raw tokens)
func NewTelegramClient(logger *slog.Logger) *TelegramService {
	return &TelegramService{
		ApiService: apiService.NewAPIService("https://api.telegram.org/", logger),
		Logger:     logger.WithGroup("telegramService"),
	}
}

func (s *TelegramService) SendMessageWithMarkup(token, chatID, message string, replyMarkup *schema.InlineKeyboardMarkup) error {
	resp, err := s.ApiService.Post(fmt.Sprintf("bot%s/sendMessage", token), schema.SendMessage{
		ChatID:    chatID,
		Text:      message,
		ParseMode: "HTML",
		LinkPreviewOptions: &schema.LinkPreviewOptions{
			IsDisabled: true,
		},
		ReplyMarkup: replyMarkup,
	}, nil, nil)

	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram api error: status %d", resp.StatusCode)
	}

	return nil
}

func (s *TelegramService) SendMessage(token, chatID, message string) error {
	return s.SendMessageWithMarkup(token, chatID, message, nil)
}

func (s *TelegramService) SendNotification(message string) error {
	if s.ApiKey == "" || s.ChatID == "" {
		return ErrTelegramNotConfigured
	}
	return s.SendMessage(s.ApiKey, s.ChatID, message)
}

func (s *TelegramService) SendEpisodeSnatched(input schema.EpisodeSnatchedTemplateInput) error {
	if s.ApiKey == "" || s.ChatID == "" {
		return ErrTelegramNotConfigured
	}

	msg := schema.FormatEpisodeSnatched(input)

	var markup *schema.InlineKeyboardMarkup
	if input.ReleaseURL != "" && (strings.HasPrefix(input.ReleaseURL, "http://") || strings.HasPrefix(input.ReleaseURL, "https://")) {
		markup = &schema.InlineKeyboardMarkup{
			InlineKeyboard: [][]schema.InlineKeyboardButton{
				{
					{
						Text: "🔗 View Release",
						URL:  input.ReleaseURL,
					},
				},
			},
		}
	}

	return s.SendMessageWithMarkup(s.ApiKey, s.ChatID, msg, markup)
}

func (s *TelegramService) SendEpisodeDownloaded(input schema.EpisodeDownloadedTemplateInput) error {
	msg := schema.FormatEpisodeDownloaded(input)
	return s.SendNotification(msg)
}

func (s *TelegramService) SendDailySummary(input schema.DailySummaryTemplateInput) error {
	msg := schema.FormatDailySummary(input)
	return s.SendNotification(msg)
}

func (s *TelegramService) SendTestNotification() error {
	msg := schema.FormatTestNotification()
	return s.SendNotification(msg)
}

func (s *TelegramService) GetLatestChatID(token string) (string, error) {
	var response schema.GetUpdatesResponse
	endpoint := fmt.Sprintf("bot%s/getUpdates", token)
	if err := s.ApiService.Get(endpoint, &response); err != nil {
		return "", fmt.Errorf("failed to fetch updates from Telegram: %w", err)
	}

	if !response.Ok {
		if response.Description != "" {
			return "", fmt.Errorf("telegram API error: %s", response.Description)
		}
		return "", errors.New("telegram API returned not ok")
	}

	if len(response.Result) == 0 {
		return "", errors.New("no messages found. Please send a message (e.g. /start) to your bot on Telegram first, then try again")
	}

	for i := len(response.Result) - 1; i >= 0; i-- {
		u := response.Result[i]
		if u.Message != nil && u.Message.Chat.ID != 0 {
			return strconv.FormatInt(u.Message.Chat.ID, 10), nil
		}
		if u.EditedMessage != nil && u.EditedMessage.Chat.ID != 0 {
			return strconv.FormatInt(u.EditedMessage.Chat.ID, 10), nil
		}
		if u.ChannelPost != nil && u.ChannelPost.Chat.ID != 0 {
			return strconv.FormatInt(u.ChannelPost.Chat.ID, 10), nil
		}
		if u.EditedChannelPost != nil && u.EditedChannelPost.Chat.ID != 0 {
			return strconv.FormatInt(u.EditedChannelPost.Chat.ID, 10), nil
		}
		if u.MyChatMember != nil && u.MyChatMember.Chat.ID != 0 {
			return strconv.FormatInt(u.MyChatMember.Chat.ID, 10), nil
		}
		if u.CallbackQuery != nil && u.CallbackQuery.Message != nil && u.CallbackQuery.Message.Chat.ID != 0 {
			return strconv.FormatInt(u.CallbackQuery.Message.Chat.ID, 10), nil
		}
	}

	return "", errors.New("no valid chat ID found in recent Telegram updates")
}
