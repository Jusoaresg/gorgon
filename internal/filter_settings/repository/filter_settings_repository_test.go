package repository

import (
	"testing"

	"github.com/jusoaresg/gorgon/internal/filter_settings/model"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterSettingsRepository_GetReturnsDefaultsWhenEmpty(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()
	repo := NewFilterSettingsRepository(db)

	got, err := repo.Get()
	assert.NoError(t, err)
	assert.True(t, got.UseAliases)
	assert.True(t, got.OnlyLatin)
	assert.Nil(t, got.DefaultFilterProfileID)
}

func TestFilterSettingsRepository_GetFallsBackToDefaultsForMissingKeys(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()
	repo := NewFilterSettingsRepository(db)

	_, err := db.Exec(`INSERT INTO app_settings (key, value) VALUES ('filters', '{"default_filter_profile_id":null}')`)
	assert.NoError(t, err)

	got, err := repo.Get()
	assert.NoError(t, err)
	assert.True(t, got.UseAliases)
	assert.True(t, got.OnlyLatin)
	assert.Nil(t, got.DefaultFilterProfileID)
}

func TestFilterSettingsRepository_SaveAndGet(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()
	repo := NewFilterSettingsRepository(db)

	profileID := int64(3)
	settings := model.FilterSettings{
		DefaultFilterProfileID: &profileID,
		UseAliases:             false,
		OnlyLatin:              false,
	}

	assert.NoError(t, repo.Save(settings))

	got, err := repo.Get()
	assert.NoError(t, err)
	assert.NotNil(t, got.DefaultFilterProfileID)
	assert.Equal(t, profileID, *got.DefaultFilterProfileID)
	assert.False(t, got.UseAliases)
	assert.False(t, got.OnlyLatin)

	// Verify individual keys in app_settings table
	var rows []struct {
		Key      string `db:"key"`
		Value    string `db:"value"`
		Category string `db:"category"`
	}
	err = db.Select(&rows, "SELECT key, value, category FROM app_settings WHERE category = 'filters' ORDER BY key")
	require.NoError(t, err)
	assert.Len(t, rows, 3)

	m := make(map[string]string)
	for _, r := range rows {
		m[r.Key] = r.Value
		assert.Equal(t, "filters", r.Category)
	}

	assert.Equal(t, "3", m[keyDefaultFilterProfileID])
	assert.Equal(t, "false", m[keyUseAliases])
	assert.Equal(t, "false", m[keyOnlyLatin])

	// Legacy 'filters' JSON row should not exist
	var legacyCount int
	err = db.Get(&legacyCount, "SELECT COUNT(*) FROM app_settings WHERE key = 'filters'")
	require.NoError(t, err)
	assert.Equal(t, 0, legacyCount)
}

func TestFilterSettingsRepository_SaveOverwrites(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()
	repo := NewFilterSettingsRepository(db)

	assert.NoError(t, repo.Save(model.FilterSettings{UseAliases: false, OnlyLatin: true}))
	assert.NoError(t, repo.Save(model.FilterSettings{UseAliases: true, OnlyLatin: true}))

	got, err := repo.Get()
	assert.NoError(t, err)
	assert.True(t, got.UseAliases)
}
