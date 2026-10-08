package cron

import (
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
)

const TvMazeLastUpdateKey = "tvmaze_last_update"

func GetLastShowsUpdateTime(db *sqlx.DB) (time.Time, error) {
	if db == nil {
		return time.Time{}, nil
	}

	var valStr string
	query := "SELECT value FROM app_settings WHERE key = ? LIMIT 1"
	if err := db.Get(&valStr, query, TvMazeLastUpdateKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}

	ts, err := strconv.ParseInt(valStr, 10, 64)
	if err != nil {
		return time.Time{}, err
	}

	return time.Unix(ts, 0), nil
}

func SetLastShowsUpdateTime(db *sqlx.DB, t time.Time) error {
	if db == nil {
		return nil
	}

	valStr := strconv.FormatInt(t.Unix(), 10)
	query := `
		INSERT INTO app_settings (key, value, category, updated_at)
		VALUES (?, ?, 'scheduler', unixepoch())
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`
	_, err := db.Exec(query, TvMazeLastUpdateKey, valStr)
	return err
}
