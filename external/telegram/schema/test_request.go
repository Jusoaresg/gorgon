package schema

type TestTelegramRequest struct {
	TelegramBotApiKey *string `json:"telegramBotApiKey"`
	TelegramChatID    *string `json:"telegramChatID"`
}
