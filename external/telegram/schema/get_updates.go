package schema

type TelegramUpdateChat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type,omitempty"`
	Title     string `json:"title,omitempty"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
}

type TelegramUpdateMessage struct {
	MessageID int64              `json:"message_id"`
	Chat      TelegramUpdateChat `json:"chat"`
	Text      string             `json:"text,omitempty"`
}

type TelegramCallbackQuery struct {
	Message *TelegramUpdateMessage `json:"message,omitempty"`
}

type TelegramMyChatMember struct {
	Chat TelegramUpdateChat `json:"chat"`
}

type TelegramUpdate struct {
	UpdateID          int64                  `json:"update_id"`
	Message           *TelegramUpdateMessage `json:"message,omitempty"`
	EditedMessage     *TelegramUpdateMessage `json:"edited_message,omitempty"`
	ChannelPost       *TelegramUpdateMessage `json:"channel_post,omitempty"`
	EditedChannelPost *TelegramUpdateMessage `json:"edited_channel_post,omitempty"`
	MyChatMember      *TelegramMyChatMember  `json:"my_chat_member,omitempty"`
	CallbackQuery     *TelegramCallbackQuery `json:"callback_query,omitempty"`
}

type GetUpdatesResponse struct {
	Ok          bool             `json:"ok"`
	Result      []TelegramUpdate `json:"result"`
	Description string           `json:"description,omitempty"`
	ErrorCode   int              `json:"error_code,omitempty"`
}
