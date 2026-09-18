package repository

import (
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/internal/show_settings/model"
)

var ErrShowSettingsNotFound = errors.New("show settings not found")

type ShowSettingsRepositoryInterface interface {
	Upsert(settings model.ShowSettings) error
	GetByShowID(showID int64) (model.ShowSettings, error)
	DeleteByShowID(showID int64) error
}

type ShowSettingsRepository struct {
	db *sqlx.DB
}

func NewShowSettingsRepository(db *sqlx.DB) ShowSettingsRepository {
	return ShowSettingsRepository{
		db: db,
	}
}

func (s *ShowSettingsRepository) Upsert(settings model.ShowSettings) error {
	now := time.Now().Unix()
	showType := settings.ShowType
	if showType == "" {
		showType = model.ShowTypeStandard
	}

	_, err := s.db.Exec(`
		INSERT INTO show_settings (show_id, filter_profile_id, show_type, use_aliases, only_latin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(show_id) DO UPDATE SET
			filter_profile_id = excluded.filter_profile_id,
			show_type = excluded.show_type,
			use_aliases = excluded.use_aliases,
			only_latin = excluded.only_latin,
			updated_at = excluded.updated_at
	`,
		settings.ShowID,
		settings.FilterProfileID,
		showType,
		settings.UseAliases,
		settings.OnlyLatin,
		now,
		now,
	)
	return err
}

func (s *ShowSettingsRepository) GetByShowID(showID int64) (model.ShowSettings, error) {
	var settings model.ShowSettings
	if err := s.db.Get(&settings, "SELECT * FROM show_settings WHERE show_id = ? LIMIT 1", showID); err != nil {
		return model.ShowSettings{}, err
	}
	if settings.ShowType == "" {
		settings.ShowType = model.ShowTypeStandard
	}
	return settings, nil
}

func (s *ShowSettingsRepository) DeleteByShowID(showID int64) error {
	result, err := s.db.Exec("DELETE FROM show_settings WHERE show_id = ?", showID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrShowSettingsNotFound
	}

	return nil
}

var _ ShowSettingsRepositoryInterface = (*ShowSettingsRepository)(nil)
