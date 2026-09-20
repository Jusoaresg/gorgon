package schemas

import (
	"encoding/json"
	"strings"
)

type ConfigFile struct {
	ProwlarrApiKey string `json:"prowlarrApiKey"`
	ProwlarrHost   string `json:"prowlarrHost"`
	ProwlarrPort   string `json:"prowlarrPort"`

	QBittorrentHost           string `json:"qBittorrentHost"`
	QBittorrentPort           string `json:"qBittorrentPort"`
	QBittorrentUsername       string `json:"qBittorrentUsername"`
	QBittorrentPassword       string `json:"qBittorrentPassword"`
	QBittorrentDownloadFolder string `json:"qBittorrentDownloadFolder"`

	DefaultShowInfoFolder string `json:"defaultShowInfoFolder"`
	ShowsFolder           string `json:"showsFolder"`
	TelegramBotApiKey     string `json:"telegramBotApiKey"`
	TelegramChatID        string `json:"telegramChatID"`

	TelegramDailySummaryEnabled bool   `json:"telegramDailySummaryEnabled"`
	TelegramDailySummaryTime    string `json:"telegramDailySummaryTime"`
	TelegramNotifyEmptySummary  bool   `json:"telegramNotifyEmptySummary"`

	Timezone string `json:"timezone"`
}

type FlexBool bool

func (b *FlexBool) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), "\"")
	switch strings.ToLower(s) {
	case "true", "1", "on", "yes":
		*b = true
	case "false", "0", "off", "no", "null", "":
		*b = false
	default:
		var v bool
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		*b = FlexBool(v)
	}
	return nil
}

// NOTE: For patch route
type UpdateConfigInput struct {
	ProwlarrApiKey *string `json:"prowlarrApiKey"`
	ProwlarrHost   *string `json:"prowlarrHost"`
	ProwlarrPort   *string `json:"prowlarrPort"`

	QBittorrentHost           *string `json:"qBittorrentHost"`
	QBittorrentPort           *string `json:"qBittorrentPort"`
	QBittorrentUsername       *string `json:"qBittorrentUsername"`
	QBittorrentPassword       *string `json:"qBittorrentPassword"`
	QBittorrentDownloadFolder *string `json:"qBittorrentDownloadFolder"`

	DefaultShowInfoFolder *string `json:"defaultShowInfoFolder"`
	ShowsFolder           *string `json:"showsFolder"`
	TelegramBotApiKey     *string `json:"telegramBotApiKey"`
	TelegramChatID        *string `json:"telegramChatID"`

	TelegramDailySummaryEnabled *FlexBool `json:"telegramDailySummaryEnabled"`
	TelegramDailySummaryTime    *string   `json:"telegramDailySummaryTime"`
	TelegramNotifyEmptySummary  *FlexBool `json:"telegramNotifyEmptySummary"`

	Timezone *string `json:"timezone"`
}

func setString(dst *string, src *string) {
	if src != nil {
		*dst = *src
	}
}

func setBool(dst *bool, src *bool) {
	if src != nil {
		*dst = *src
	}
}

func setFlexBool(dst *bool, src *FlexBool) {
	if src != nil {
		*dst = bool(*src)
	}
}

func (c *ConfigFile) Apply(input *UpdateConfigInput) {
	setString(&c.ProwlarrApiKey, input.ProwlarrApiKey)
	setString(&c.ProwlarrHost, input.ProwlarrHost)
	setString(&c.ProwlarrPort, input.ProwlarrPort)

	setString(&c.QBittorrentHost, input.QBittorrentHost)
	setString(&c.QBittorrentPort, input.QBittorrentPort)
	setString(&c.QBittorrentUsername, input.QBittorrentUsername)
	setString(&c.QBittorrentPassword, input.QBittorrentPassword)
	setString(&c.QBittorrentDownloadFolder, input.QBittorrentDownloadFolder)

	setString(&c.DefaultShowInfoFolder, input.DefaultShowInfoFolder)
	setString(&c.ShowsFolder, input.ShowsFolder)

	setString(&c.TelegramBotApiKey, input.TelegramBotApiKey)
	setString(&c.TelegramChatID, input.TelegramChatID)

	setFlexBool(&c.TelegramDailySummaryEnabled, input.TelegramDailySummaryEnabled)
	setString(&c.TelegramDailySummaryTime, input.TelegramDailySummaryTime)
	setFlexBool(&c.TelegramNotifyEmptySummary, input.TelegramNotifyEmptySummary)

	setString(&c.Timezone, input.Timezone)
}
