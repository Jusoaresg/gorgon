package schema

type LinkPreviewOptions struct {
	IsDisabled bool `json:"is_disabled"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	URL          string `json:"url,omitempty"`
	CallbackData string `json:"callback_data,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type KeyboardButton struct {
	Text string `json:"text"`
}

type ReplyKeyboardMarkup struct {
	Keyboard              [][]KeyboardButton `json:"keyboard"`
	ResizeKeyboard        bool               `json:"resize_keyboard,omitempty"`
	IsPersistent          bool               `json:"is_persistent,omitempty"`
	InputFieldPlaceholder string             `json:"input_field_placeholder,omitempty"`
}

type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

type SetMyCommandsRequest struct {
	Commands []BotCommand `json:"commands"`
}

type SendMessage struct {
	ChatID             string              `json:"chat_id"`
	Text               string              `json:"text"`
	ParseMode          string              `json:"parse_mode"`
	LinkPreviewOptions *LinkPreviewOptions `json:"link_preview_options,omitempty"`
	ReplyMarkup        any                 `json:"reply_markup,omitempty"`
}

type AnswerCallbackQueryRequest struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
}

type EditMessageReplyMarkupRequest struct {
	ChatID      string                `json:"chat_id"`
	MessageID   int64                 `json:"message_id"`
	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

type EditMessageTextRequest struct {
	ChatID             string                `json:"chat_id"`
	MessageID          int64                 `json:"message_id"`
	Text               string                `json:"text"`
	ParseMode          string                `json:"parse_mode"`
	LinkPreviewOptions *LinkPreviewOptions   `json:"link_preview_options,omitempty"`
	ReplyMarkup        *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}
