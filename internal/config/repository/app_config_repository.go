package repository

import (
	"strconv"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/pkg/schemas"
)

const (
	KeyProwlarrApiKey              = "prowlarr.api_key"
	KeyProwlarrHost                = "prowlarr.host"
	KeyProwlarrPort                = "prowlarr.port"
	KeyQBittorrentHost             = "qbittorrent.host"
	KeyQBittorrentPort             = "qbittorrent.port"
	KeyQBittorrentUsername         = "qbittorrent.username"
	KeyQBittorrentPassword         = "qbittorrent.password"
	KeyQBittorrentDownloadFolder   = "qbittorrent.download_folder"
	KeyStorageShowsFolder          = "storage.shows_folder"
	KeyStorageDefaultShowInfo      = "storage.default_show_info_folder"
	KeyTelegramBotApiKey           = "telegram.bot_api_key"
	KeyTelegramChatID              = "telegram.chat_id"
	KeyTelegramDailySummaryEnabled = "telegram.daily_summary_enabled"
	KeyTelegramDailySummaryTime    = "telegram.daily_summary_time"
	KeyTelegramNotifyEmptySummary  = "telegram.notify_empty_summary"
)

const (
	CategoryProwlarr    = "prowlarr"
	CategoryQBittorrent = "qbittorrent"
	CategoryStorage     = "storage"
	CategoryTelegram    = "telegram"
)

var KeyCategoryMap = map[string]string{
	KeyProwlarrApiKey:              CategoryProwlarr,
	KeyProwlarrHost:                CategoryProwlarr,
	KeyProwlarrPort:                CategoryProwlarr,
	KeyQBittorrentHost:             CategoryQBittorrent,
	KeyQBittorrentPort:             CategoryQBittorrent,
	KeyQBittorrentUsername:         CategoryQBittorrent,
	KeyQBittorrentPassword:         CategoryQBittorrent,
	KeyQBittorrentDownloadFolder:   CategoryQBittorrent,
	KeyStorageShowsFolder:          CategoryStorage,
	KeyStorageDefaultShowInfo:      CategoryStorage,
	KeyTelegramBotApiKey:           CategoryTelegram,
	KeyTelegramChatID:              CategoryTelegram,
	KeyTelegramDailySummaryEnabled: CategoryTelegram,
	KeyTelegramDailySummaryTime:    CategoryTelegram,
	KeyTelegramNotifyEmptySummary:  CategoryTelegram,
}

type SettingRow struct {
	Key       string `db:"key"`
	Value     string `db:"value"`
	Category  string `db:"category"`
	UpdatedAt int64  `db:"updated_at"`
}

type AppConfigRepositoryInterface interface {
	GetAll() (map[string]string, error)
	GetConfig() (*schemas.ConfigFile, error)
	SaveConfig(config *schemas.ConfigFile) error
	SaveKey(key, value, category string) error
	HasAnyConfig() (bool, error)
}

type AppConfigRepository struct {
	db       *sqlx.DB
	inDocker bool
}

func NewAppConfigRepository(db *sqlx.DB, inDocker bool) *AppConfigRepository {
	return &AppConfigRepository{
		db:       db,
		inDocker: inDocker,
	}
}

func (r *AppConfigRepository) GetAll() (map[string]string, error) {
	var rows []SettingRow
	err := r.db.Select(&rows, "SELECT key, value, category, updated_at FROM app_settings")
	if err != nil {
		return nil, err
	}

	result := make(map[string]string, len(rows))
	for _, row := range rows {
		result[row.Key] = row.Value
	}
	return result, nil
}

func (r *AppConfigRepository) HasAnyConfig() (bool, error) {
	var count int
	err := r.db.Get(&count, `
		SELECT COUNT(*) FROM app_settings
		WHERE key IN (
			'prowlarr.api_key', 'prowlarr.host', 'telegram.bot_api_key', 'qbittorrent.host'
		)
	`)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *AppConfigRepository) GetConfig() (*schemas.ConfigFile, error) {
	cfg := DefaultConfigFile(r.inDocker)
	m, err := r.GetAll()
	if err != nil {
		return &cfg, nil
	}

	if val, ok := m[KeyProwlarrApiKey]; ok {
		cfg.ProwlarrApiKey = val
	}
	if val, ok := m[KeyProwlarrHost]; ok {
		cfg.ProwlarrHost = val
	}
	if val, ok := m[KeyProwlarrPort]; ok {
		cfg.ProwlarrPort = val
	}
	if val, ok := m[KeyQBittorrentHost]; ok {
		cfg.QBittorrentHost = val
	}
	if val, ok := m[KeyQBittorrentPort]; ok {
		cfg.QBittorrentPort = val
	}
	if val, ok := m[KeyQBittorrentUsername]; ok {
		cfg.QBittorrentUsername = val
	}
	if val, ok := m[KeyQBittorrentPassword]; ok {
		cfg.QBittorrentPassword = val
	}
	if val, ok := m[KeyQBittorrentDownloadFolder]; ok {
		cfg.QBittorrentDownloadFolder = val
	}
	if val, ok := m[KeyStorageShowsFolder]; ok {
		cfg.ShowsFolder = val
	}
	if val, ok := m[KeyStorageDefaultShowInfo]; ok {
		cfg.DefaultShowInfoFolder = val
	}
	if val, ok := m[KeyTelegramBotApiKey]; ok {
		cfg.TelegramBotApiKey = val
	}
	if val, ok := m[KeyTelegramChatID]; ok {
		cfg.TelegramChatID = val
	}
	if val, ok := m[KeyTelegramDailySummaryEnabled]; ok {
		cfg.TelegramDailySummaryEnabled = parseBool(val, true)
	}
	if val, ok := m[KeyTelegramDailySummaryTime]; ok {
		cfg.TelegramDailySummaryTime = val
	}
	if val, ok := m[KeyTelegramNotifyEmptySummary]; ok {
		cfg.TelegramNotifyEmptySummary = parseBool(val, true)
	}

	return &cfg, nil
}

func (r *AppConfigRepository) SaveConfig(config *schemas.ConfigFile) error {
	items := map[string]string{
		KeyProwlarrApiKey:              config.ProwlarrApiKey,
		KeyProwlarrHost:                config.ProwlarrHost,
		KeyProwlarrPort:                config.ProwlarrPort,
		KeyQBittorrentHost:             config.QBittorrentHost,
		KeyQBittorrentPort:             config.QBittorrentPort,
		KeyQBittorrentUsername:         config.QBittorrentUsername,
		KeyQBittorrentPassword:         config.QBittorrentPassword,
		KeyQBittorrentDownloadFolder:   config.QBittorrentDownloadFolder,
		KeyStorageShowsFolder:          config.ShowsFolder,
		KeyStorageDefaultShowInfo:      config.DefaultShowInfoFolder,
		KeyTelegramBotApiKey:           config.TelegramBotApiKey,
		KeyTelegramChatID:              config.TelegramChatID,
		KeyTelegramDailySummaryEnabled: strconv.FormatBool(config.TelegramDailySummaryEnabled),
		KeyTelegramDailySummaryTime:    config.TelegramDailySummaryTime,
		KeyTelegramNotifyEmptySummary:  strconv.FormatBool(config.TelegramNotifyEmptySummary),
	}

	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO app_settings (key, value, category, updated_at)
		VALUES (?, ?, ?, unixepoch())
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			category = excluded.category,
			updated_at = unixepoch()
	`

	for key, value := range items {
		cat := KeyCategoryMap[key]
		if _, err := tx.Exec(query, key, value, cat); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *AppConfigRepository) SaveKey(key, value, category string) error {
	query := `
		INSERT INTO app_settings (key, value, category, updated_at)
		VALUES (?, ?, ?, unixepoch())
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			category = excluded.category,
			updated_at = unixepoch()
	`
	_, err := r.db.Exec(query, key, value, category)
	return err
}

func parseBool(val string, defaultVal bool) bool {
	if b, err := strconv.ParseBool(val); err == nil {
		return b
	}
	switch val {
	case "1", "yes", "on":
		return true
	case "0", "no", "off":
		return false
	default:
		return defaultVal
	}
}

func DefaultConfigFile(inDocker bool) schemas.ConfigFile {
	cfg := schemas.ConfigFile{
		ProwlarrApiKey:              "",
		ProwlarrHost:                "",
		ProwlarrPort:                "",
		QBittorrentHost:             "",
		QBittorrentPort:             "",
		QBittorrentUsername:         "",
		QBittorrentPassword:         "",
		QBittorrentDownloadFolder:   "downloads",
		DefaultShowInfoFolder:       "shows",
		ShowsFolder:                 "/home/user/Videos/shows",
		TelegramBotApiKey:           "",
		TelegramChatID:              "",
		TelegramDailySummaryEnabled: true,
		TelegramDailySummaryTime:    "08:00",
		TelegramNotifyEmptySummary:  true,
	}

	if inDocker {
		cfg.QBittorrentDownloadFolder = "/downloads"
		cfg.ShowsFolder = "/shows"
		cfg.DefaultShowInfoFolder = "/app/assets/shows"
		cfg.ProwlarrHost = "http://gorgon-prowlarr"
		cfg.ProwlarrPort = "9696"
		cfg.QBittorrentHost = "http://gorgon-qbittorrent"
		cfg.QBittorrentPort = "9191"
		cfg.QBittorrentUsername = "admin"
	}

	return cfg
}

var _ AppConfigRepositoryInterface = (*AppConfigRepository)(nil)
