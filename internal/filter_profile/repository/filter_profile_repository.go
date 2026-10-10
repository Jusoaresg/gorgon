package repository

import (
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/internal/filter_profile/model"
)

var ErrFilterProfileNotFound = errors.New("filter profile not found")

type FilterProfileRepositoryInterface interface {
	Create(profile model.FilterProfile, patterns []model.FilterPattern) (int64, error)
	Update(profile model.FilterProfile, patterns []model.FilterPattern) error
	Delete(id int64) error
	GetByID(id int64) (model.FilterProfile, []model.FilterPattern, error)
	List() ([]model.FilterProfile, error)
}

type FilterProfileRepository struct {
	db *sqlx.DB
}

func NewFilterProfileRepository(db *sqlx.DB) FilterProfileRepository {
	return FilterProfileRepository{
		db: db,
	}
}

func (s *FilterProfileRepository) Create(profile model.FilterProfile, patterns []model.FilterPattern) (int64, error) {
	tx, err := s.db.Beginx()
	if err != nil {
		return 0, err
	}

	now := time.Now().Unix()
	profile.CreatedAt = now
	profile.UpdatedAt = now

	result, err := tx.NamedExec(`
		INSERT INTO filter_profiles (name, upgrade_until_score, created_at, updated_at)
		VALUES (:name, :upgrade_until_score, :created_at, :updated_at)
	`, profile)
	if err != nil {
		tx.Rollback()
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := s.insertPatternsTx(tx, id, patterns); err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return id, nil
}

func (s *FilterProfileRepository) Update(profile model.FilterProfile, patterns []model.FilterPattern) error {
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}

	profile.UpdatedAt = time.Now().Unix()

	result, err := tx.NamedExec(`
		UPDATE filter_profiles
		SET name = :name, upgrade_until_score = :upgrade_until_score, updated_at = :updated_at
		WHERE id = :id
	`, profile)
	if err != nil {
		tx.Rollback()
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		tx.Rollback()
		return err
	}
	if rows == 0 {
		tx.Rollback()
		return ErrFilterProfileNotFound
	}

	if _, err := tx.Exec("DELETE FROM filter_patterns WHERE profile_id = ?", profile.ID); err != nil {
		tx.Rollback()
		return err
	}

	if err := s.insertPatternsTx(tx, profile.ID, patterns); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

func (s *FilterProfileRepository) Delete(id int64) error {
	result, err := s.db.Exec("DELETE FROM filter_profiles WHERE id = ?", id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrFilterProfileNotFound
	}

	return nil
}

func (s *FilterProfileRepository) GetByID(id int64) (model.FilterProfile, []model.FilterPattern, error) {
	var profile model.FilterProfile
	if err := s.db.Get(&profile, "SELECT * FROM filter_profiles WHERE id = ? LIMIT 1", id); err != nil {
		return model.FilterProfile{}, nil, err
	}

	patterns, err := s.listPatterns(id)
	if err != nil {
		return model.FilterProfile{}, nil, err
	}

	return profile, patterns, nil
}

func (s *FilterProfileRepository) List() ([]model.FilterProfile, error) {
	var profiles []model.FilterProfile
	if err := s.db.Select(&profiles, "SELECT * FROM filter_profiles ORDER BY name COLLATE NOCASE"); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (s *FilterProfileRepository) insertPatternsTx(tx *sqlx.Tx, profileID int64, patterns []model.FilterPattern) error {
	for position, pattern := range patterns {
		pattern.ProfileID = profileID
		pattern.Position = position
		if _, err := tx.NamedExec(`
			INSERT INTO filter_patterns (profile_id, kind, pattern, score, position)
			VALUES (:profile_id, :kind, :pattern, :score, :position)
		`, pattern); err != nil {
			return err
		}
	}
	return nil
}

func (s *FilterProfileRepository) listPatterns(profileID int64) ([]model.FilterPattern, error) {
	var patterns []model.FilterPattern
	if err := s.db.Select(&patterns, "SELECT * FROM filter_patterns WHERE profile_id = ? ORDER BY position ASC", profileID); err != nil {
		return nil, err
	}
	return patterns, nil
}

var _ FilterProfileRepositoryInterface = (*FilterProfileRepository)(nil)
