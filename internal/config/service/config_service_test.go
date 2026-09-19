package service_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jusoaresg/gorgon/internal/config/repository"
	"github.com/jusoaresg/gorgon/internal/config/service"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigService_GetAndInitialState(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	repo := repository.NewAppConfigRepository(db, false)
	svc, err := service.NewConfigService(repo, nil)
	require.NoError(t, err)

	cfg := svc.Get()
	assert.Equal(t, "/home/user/Videos/shows", cfg.ShowsFolder)
	assert.True(t, cfg.TelegramDailySummaryEnabled)
	assert.Equal(t, "08:00", cfg.TelegramDailySummaryTime)
}

func TestConfigService_Update(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	repo := repository.NewAppConfigRepository(db, false)
	svc, err := service.NewConfigService(repo, nil)
	require.NoError(t, err)

	newHost := "http://prowlarr.local"
	newPort := "9696"
	newSummaryTime := "12:00"
	var falseFlex schemas.FlexBool = false

	updated, err := svc.Update(&schemas.UpdateConfigInput{
		ProwlarrHost:                &newHost,
		ProwlarrPort:                &newPort,
		TelegramDailySummaryTime:    &newSummaryTime,
		TelegramDailySummaryEnabled: &falseFlex,
	})
	require.NoError(t, err)
	assert.Equal(t, "http://prowlarr.local", updated.ProwlarrHost)
	assert.Equal(t, "9696", updated.ProwlarrPort)
	assert.Equal(t, "12:00", updated.TelegramDailySummaryTime)
	assert.False(t, updated.TelegramDailySummaryEnabled)

	// Verify persistence in repository / new service instance
	svc2, err := service.NewConfigService(repo, nil)
	require.NoError(t, err)
	cfg2 := svc2.Get()
	assert.Equal(t, "http://prowlarr.local", cfg2.ProwlarrHost)
	assert.Equal(t, "9696", cfg2.ProwlarrPort)
	assert.False(t, cfg2.TelegramDailySummaryEnabled)
}

func TestConfigService_ConcurrentReadWrite(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	repo := repository.NewAppConfigRepository(db, false)
	svc, err := service.NewConfigService(repo, nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	// 50 concurrent readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = svc.Get()
			}
		}()
	}

	// 5 concurrent writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			token := "token"
			_, _ = svc.Update(&schemas.UpdateConfigInput{
				TelegramBotApiKey: &token,
			})
		}(i)
	}

	wg.Wait()
}

func TestConfigService_MigrateLegacyConfigFile(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	repo := repository.NewAppConfigRepository(db, false)
	svc, err := service.NewConfigService(repo, nil)
	require.NoError(t, err)

	// Create a temporary legacy config.json
	tempDir := t.TempDir()
	legacyFile := filepath.Join(tempDir, "config.json")
	legacyData := schemas.ConfigFile{
		TelegramBotApiKey: "legacy-bot-key-12345",
		TelegramChatID:    "987654321",
		ProwlarrHost:      "http://legacy-prowlarr:9696",
	}
	bytes, err := json.Marshal(legacyData)
	require.NoError(t, err)
	err = os.WriteFile(legacyFile, bytes, 0644)
	require.NoError(t, err)

	// Migrate
	err = svc.MigrateLegacyConfigFile(legacyFile)
	require.NoError(t, err)

	// Config should now be loaded in service
	cfg := svc.Get()
	assert.Equal(t, "legacy-bot-key-12345", cfg.TelegramBotApiKey)
	assert.Equal(t, "987654321", cfg.TelegramChatID)
	assert.Equal(t, "http://legacy-prowlarr:9696", cfg.ProwlarrHost)

	// config.json should be moved to config.json.bak
	_, err = os.Stat(legacyFile)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(legacyFile + ".bak")
	assert.NoError(t, err)
}
