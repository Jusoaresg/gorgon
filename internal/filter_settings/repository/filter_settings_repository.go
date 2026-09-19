package repository

import (
	"encoding/json"
	"strconv"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/internal/filter_settings/model"
)

const (
	categoryFilters           = "filters"
	keyDefaultFilterProfileID = "filters.default_filter_profile_id"
	keyUseAliases             = "filters.use_aliases"
	keyOnlyLatin              = "filters.only_latin"
)

type FilterSettingsRepositoryInterface interface {
	Get() (model.FilterSettings, error)
	Save(settings model.FilterSettings) error
}

type FilterSettingsRepository struct {
	db *sqlx.DB
}

func NewFilterSettingsRepository(db *sqlx.DB) FilterSettingsRepository {
	return FilterSettingsRepository{
		db: db,
	}
}

func (s *FilterSettingsRepository) Get() (model.FilterSettings, error) {
	settings := model.DefaultFilterSettings()

	var rows []struct {
		Key   string `db:"key"`
		Value string `db:"value"`
	}
	err := s.db.Select(&rows, "SELECT key, value FROM app_settings WHERE key LIKE 'filters%'")
	if err != nil || len(rows) == 0 {
		return settings, nil
	}

	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.Key] = r.Value
	}

	// 1. Check if individual keys exist
	hasIndividualKeys := false
	if val, ok := m[keyUseAliases]; ok {
		hasIndividualKeys = true
		settings.UseAliases = parseBool(val, true)
	}
	if val, ok := m[keyOnlyLatin]; ok {
		hasIndividualKeys = true
		settings.OnlyLatin = parseBool(val, true)
	}
	if val, ok := m[keyDefaultFilterProfileID]; ok {
		hasIndividualKeys = true
		if val == "" || val == "null" {
			settings.DefaultFilterProfileID = nil
		} else if id, err := strconv.ParseInt(val, 10, 64); err == nil {
			settings.DefaultFilterProfileID = &id
		}
	}

	if hasIndividualKeys {
		return settings, nil
	}

	// 2. Fallback to legacy JSON row 'filters' if it exists
	if legacyJSON, ok := m["filters"]; ok {
		var stored struct {
			DefaultFilterProfileID *int64 `json:"default_filter_profile_id"`
			UseAliases             *bool  `json:"use_aliases"`
			OnlyLatin              *bool  `json:"only_latin"`
		}
		if err := json.Unmarshal([]byte(legacyJSON), &stored); err == nil {
			if stored.DefaultFilterProfileID != nil {
				settings.DefaultFilterProfileID = stored.DefaultFilterProfileID
			}
			if stored.UseAliases != nil {
				settings.UseAliases = *stored.UseAliases
			}
			if stored.OnlyLatin != nil {
				settings.OnlyLatin = *stored.OnlyLatin
			}
			// Migrate to individual keys and remove legacy JSON row
			_ = s.Save(settings)
			_, _ = s.db.Exec("DELETE FROM app_settings WHERE key = 'filters'")
		}
	}

	return settings, nil
}

func (s *FilterSettingsRepository) Save(settings model.FilterSettings) error {
	tx, err := s.db.Beginx()
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

	var profileIDStr string
	if settings.DefaultFilterProfileID != nil {
		profileIDStr = strconv.FormatInt(*settings.DefaultFilterProfileID, 10)
	}

	if _, err := tx.Exec(query, keyDefaultFilterProfileID, profileIDStr, categoryFilters); err != nil {
		return err
	}
	if _, err := tx.Exec(query, keyUseAliases, strconv.FormatBool(settings.UseAliases), categoryFilters); err != nil {
		return err
	}
	if _, err := tx.Exec(query, keyOnlyLatin, strconv.FormatBool(settings.OnlyLatin), categoryFilters); err != nil {
		return err
	}

	// Clean up legacy JSON row if it still exists
	_, _ = tx.Exec("DELETE FROM app_settings WHERE key = 'filters'")

	return tx.Commit()
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

var _ FilterSettingsRepositoryInterface = (*FilterSettingsRepository)(nil)
