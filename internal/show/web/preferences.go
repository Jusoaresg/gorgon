package web

import (
	"strings"

	"github.com/jmoiron/sqlx"
)

const (
	CategoryUI     = "ui"
	KeyShowsSort   = "ui.shows_sort"
	KeyShowsStatus = "ui.shows_status"

	DefaultShowsSort   = "added"
	DefaultShowsStatus = ""
)

func isValidSort(s string) bool {
	switch s {
	case "name", "next", "added":
		return true
	default:
		return false
	}
}

func isValidStatus(s string) bool {
	switch strings.ToLower(s) {
	case "", "all", "running", "ended":
		return true
	default:
		return false
	}
}

// normalizeStatus returns the canonical status string expected by ShowRepo ("" for all, "Running", "Ended").
func normalizeStatus(s string) string {
	switch strings.ToLower(s) {
	case "running":
		return "Running"
	case "ended":
		return "Ended"
	default:
		return ""
	}
}

// GetShowsPreferences returns the saved sort and status from app_settings, falling back to defaults.
func GetShowsPreferences(db *sqlx.DB) (string, string) {
	if db == nil {
		return DefaultShowsSort, DefaultShowsStatus
	}

	var rows []struct {
		Key   string `db:"key"`
		Value string `db:"value"`
	}
	err := db.Select(&rows, "SELECT key, value FROM app_settings WHERE key IN (?, ?)", KeyShowsSort, KeyShowsStatus)
	if err != nil {
		return DefaultShowsSort, DefaultShowsStatus
	}

	sort := DefaultShowsSort
	status := DefaultShowsStatus
	for _, r := range rows {
		switch r.Key {
		case KeyShowsSort:
			if isValidSort(r.Value) {
				sort = r.Value
			}
		case KeyShowsStatus:
			if isValidStatus(r.Value) {
				status = normalizeStatus(r.Value)
			}
		}
	}
	return sort, status
}

// SaveShowsPreferences saves the sort and/or status to app_settings.
func SaveShowsPreferences(db *sqlx.DB, sort, status string) error {
	if db == nil {
		return nil
	}

	query := `
		INSERT INTO app_settings (key, value, category, updated_at)
		VALUES (?, ?, 'ui', unixepoch())
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`

	if isValidSort(sort) {
		if _, err := db.Exec(query, KeyShowsSort, sort); err != nil {
			return err
		}
	}

	if isValidStatus(status) {
		normStatus := normalizeStatus(status)
		if _, err := db.Exec(query, KeyShowsStatus, normStatus); err != nil {
			return err
		}
	}

	return nil
}
