package repository

import (
	"errors"
	"time"

	"github.com/jusoaresg/gorgon/internal/episode/model"

	"github.com/jmoiron/sqlx"
)

type EpisodeRepositoryInterface interface {
	Create(episode model.Episode) (int64, error)
	CreateTx(tx *sqlx.Tx, episode model.Episode) (int64, error)
	DeleteByID(id int64) error
	DeleteTx(tx *sqlx.Tx, id int64) error
	GetByID(id int64) (model.Episode, error)
	GetAllByID(ids ...int64) ([]model.Episode, error)
	List() ([]model.Episode, error)
	ListByShowID(showID int64) ([]model.Episode, error)
	ListBySeasonID(seasonId int) ([]model.Episode, error)
	ListByTracking(trackings ...string) ([]model.Episode, error)
	ListReleasedByTracking(trackings ...string) ([]model.Episode, error)
	Update(episode model.Episode) error
	UpdateTx(tx *sqlx.Tx, episode model.Episode) error
}

type EpisodeRepository struct {
	db *sqlx.DB
}

func NewEpisodeRepository(db *sqlx.DB) *EpisodeRepository {
	return &EpisodeRepository{
		db: db,
	}
}

func (s *EpisodeRepository) Create(episode model.Episode) (int64, error) {
	query := `
	INSERT INTO episodes (
		show_id, 
		season_id,
		name, 
		summary,
		type,
		number,
		season,
		airstamp,
		tracking
	) 
	VALUES (
		:show_id, 
		:season_id,
		:name, 
		:summary,
		:type,
		:number,
		:season,
		:airstamp,
		:tracking
	) 
	`
	result, err := s.db.NamedExec(query, episode)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (s *EpisodeRepository) CreateTx(tx *sqlx.Tx, episode model.Episode) (int64, error) {
	query := `
	INSERT INTO episodes (
		show_id, 
		season_id,
		name, 
		summary,
		type,
		number,
		season,
		airstamp,
		tracking
	) 
	VALUES (
		:show_id, 
		:season_id,
		:name, 
		:summary,
		:type,
		:number,
		:season,
		:airstamp,
		:tracking
	) 
	`
	result, err := tx.NamedExec(query, episode)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	return id, nil
}

func (s *EpisodeRepository) GetByID(id int64) (model.Episode, error) {
	var episode model.Episode
	if err := s.db.Get(&episode, "SELECT * FROM episodes WHERE id = ? LIMIT 1", id); err != nil {
		return model.Episode{}, err
	}
	return episode, nil
}

func (s *EpisodeRepository) GetAllByID(ids ...int64) ([]model.Episode, error) {
	var episodes []model.Episode

	if len(ids) <= 0 {
		return episodes, nil
	}

	query, args, err := sqlx.In("SELECT * FROM episodes WHERE id IN (?)", ids)
	if err != nil {
		return episodes, err
	}

	query = s.db.Rebind(query)

	err = s.db.Select(&episodes, query, args...)
	if err != nil {
		return episodes, err
	}

	return episodes, nil
}

func (s *EpisodeRepository) DeleteByID(id int64) error {
	if _, err := s.db.Exec("DELETE FROM episodes WHERE id = ?", id); err != nil {
		return err
	}
	return nil
}

func (s *EpisodeRepository) DeleteTx(tx *sqlx.Tx, id int64) error {
	if _, err := tx.Exec("DELETE FROM episodes WHERE id = ?", id); err != nil {
		return err
	}
	return nil
}

func (s *EpisodeRepository) List() ([]model.Episode, error) {
	var episodes []model.Episode
	if err := s.db.Select(&episodes, "SELECT * FROM episodes"); err != nil {
		return []model.Episode{}, err
	}
	return episodes, nil
}

func (s *EpisodeRepository) ListBySeasonID(seasonId int) ([]model.Episode, error) {
	var episodes []model.Episode

	if err := s.db.Select(&episodes, "SELECT * FROM episodes WHERE season_id = ?", seasonId); err != nil {
		return []model.Episode{}, err
	}
	return episodes, nil
}

func (s *EpisodeRepository) ListByTracking(trackings ...string) ([]model.Episode, error) {
	var episodes []model.Episode

	if len(trackings) <= 0 {
		return episodes, nil
	}

	query, args, err := sqlx.In("SELECT * FROM episodes WHERE tracking IN (?)", trackings)
	if err != nil {
		return episodes, err
	}

	query = s.db.Rebind(query)

	err = s.db.Select(&episodes, query, args...)
	if err != nil {
		return episodes, err
	}

	return episodes, nil
}

func (s *EpisodeRepository) ListReleasedByTracking(trackings ...string) ([]model.Episode, error) {
	var episodes []model.Episode

	if len(trackings) <= 0 {
		return episodes, nil
	}

	query, args, err := sqlx.In("SELECT * FROM episodes WHERE tracking IN (?) AND airstamp <= ?", trackings, time.Now().Unix())
	if err != nil {
		return episodes, err
	}

	query = s.db.Rebind(query)

	err = s.db.Select(&episodes, query, args...)
	if err != nil {
		return episodes, err
	}

	return episodes, nil
}

func (s *EpisodeRepository) ListByShowID(showID int64) ([]model.Episode, error) {
	var episodes []model.Episode
	if err := s.db.Select(&episodes, "SELECT * FROM episodes WHERE show_id = ?", showID); err != nil {
		return []model.Episode{}, err
	}
	return episodes, nil
}

func (s *EpisodeRepository) Update(episode model.Episode) error {
	query := `
	UPDATE episodes SET
		show_id = :show_id,
		season_id = :season_id,
		name = :name,
		summary = :summary,
		type = :type,
		number = :number,
		season = :season,
		airstamp = :airstamp,
		tracking = :tracking
	WHERE id = :id
	`
	result, err := s.db.NamedExec(query, episode)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("no rows updated")
	}

	return nil
}

func (s *EpisodeRepository) UpdateTx(tx *sqlx.Tx, episode model.Episode) error {
	query := `
	UPDATE episodes SET
		show_id = :show_id,
		season_id = :season_id,
		name = :name,
		summary = :summary,
		type = :type,
		number = :number,
		season = :season,
		airstamp = :airstamp,
		tracking = :tracking
	WHERE id = :id
	`
	_, err := tx.NamedExec(query, episode)
	if err != nil {
		tx.Rollback()
		return err
	}
	return nil
}

var _ EpisodeRepositoryInterface = (*EpisodeRepository)(nil)
