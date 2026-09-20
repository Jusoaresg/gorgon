package repository_test

import (
	"testing"

	"github.com/jusoaresg/gorgon/internal/config/repository"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppConfigRepository_GetConfig_Default(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	repo := repository.NewAppConfigRepository(db, false)
	cfg, err := repo.GetConfig()
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, "/home/user/Videos/shows", cfg.ShowsFolder)
	assert.True(t, cfg.TelegramDailySummaryEnabled)
	assert.Equal(t, "08:00", cfg.TelegramDailySummaryTime)
}

func TestAppConfigRepository_GetConfig_DockerDefault(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	repo := repository.NewAppConfigRepository(db, true)
	cfg, err := repo.GetConfig()
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, "/shows", cfg.ShowsFolder)
	assert.Equal(t, "http://gorgon-prowlarr", cfg.ProwlarrHost)
}

func TestAppConfigRepository_SaveAndGetConfig(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	repo := repository.NewAppConfigRepository(db, false)
	cfg := &schemas.ConfigFile{
		ProwlarrApiKey:              "test-prowlarr-key",
		ProwlarrHost:                "http://localhost:9696",
		TelegramBotApiKey:           "test-telegram-token",
		TelegramChatID:              "123456789",
		TelegramDailySummaryEnabled: false,
		TelegramDailySummaryTime:    "10:30",
		TelegramNotifyEmptySummary:  false,
		Timezone:                    "America/Sao_Paulo",
	}

	err := repo.SaveConfig(cfg)
	require.NoError(t, err)

	hasConfig, err := repo.HasAnyConfig()
	require.NoError(t, err)
	assert.True(t, hasConfig)

	// Verify values retrieved
	retrieved, err := repo.GetConfig()
	require.NoError(t, err)
	assert.Equal(t, "test-prowlarr-key", retrieved.ProwlarrApiKey)
	assert.Equal(t, "http://localhost:9696", retrieved.ProwlarrHost)
	assert.Equal(t, "test-telegram-token", retrieved.TelegramBotApiKey)
	assert.Equal(t, "123456789", retrieved.TelegramChatID)
	assert.False(t, retrieved.TelegramDailySummaryEnabled)
	assert.Equal(t, "10:30", retrieved.TelegramDailySummaryTime)
	assert.False(t, retrieved.TelegramNotifyEmptySummary)
	assert.Equal(t, "America/Sao_Paulo", retrieved.Timezone)

	// Verify categories in app_settings table
	var rows []repository.SettingRow
	err = db.Select(&rows, "SELECT key, value, category, updated_at FROM app_settings WHERE category != 'filters' ORDER BY key")
	require.NoError(t, err)
	assert.NotEmpty(t, rows)

	categoryMap := make(map[string]string)
	for _, r := range rows {
		categoryMap[r.Key] = r.Category
		assert.Greater(t, r.UpdatedAt, int64(0), "updated_at should be populated")
	}

	assert.Equal(t, "prowlarr", categoryMap[repository.KeyProwlarrHost])
	assert.Equal(t, "telegram", categoryMap[repository.KeyTelegramBotApiKey])
	assert.Equal(t, "telegram", categoryMap[repository.KeyTelegramChatID])
	assert.Equal(t, "general", categoryMap[repository.KeyGeneralTimezone])
	assert.Equal(t, "qbittorrent", categoryMap[repository.KeyQBittorrentHost])
	assert.Equal(t, "storage", categoryMap[repository.KeyStorageShowsFolder])
}

func TestAppConfigRepository_SaveKey(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	repo := repository.NewAppConfigRepository(db, false)
	err := repo.SaveKey(repository.KeyTelegramBotApiKey, "bot-token-xyz", repository.CategoryTelegram)
	require.NoError(t, err)

	cfg, err := repo.GetConfig()
	require.NoError(t, err)
	assert.Equal(t, "bot-token-xyz", cfg.TelegramBotApiKey)
}
