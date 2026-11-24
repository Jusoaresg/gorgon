package repository

import (
	"errors"

	"github.com/jusoaresg/gorgon/internal/episode_torrent/model"

	"github.com/jmoiron/sqlx"
)

type EpisodeTorrentRepository struct {
	db *sqlx.DB
}

func NewEpisodeTorrentRepository(db *sqlx.DB) *EpisodeTorrentRepository {
	return &EpisodeTorrentRepository{
		db: db,
	}
}

func (s *EpisodeTorrentRepository) Upsert(t model.EpisodeTorrent) (int64, error) {
	query := `
	INSERT INTO episode_torrents (
		episode_id,
		hash,
		title,
		indexer,
		info_url,
		publish_date,
		score,
		created_at,
		is_manual,
		previous_hash,
		previous_title,
		previous_indexer,
		previous_info_url,
		previous_publish_date,
		previous_score
	)
	VALUES (
		:episode_id,
		:hash,
		:title,
		:indexer,
		:info_url,
		:publish_date,
		:score,
		:created_at,
		:is_manual,
		:previous_hash,
		:previous_title,
		:previous_indexer,
		:previous_info_url,
		:previous_publish_date,
		:previous_score
	)
	ON CONFLICT(episode_id) DO UPDATE SET
		hash = :hash,
		title = :title,
		indexer = :indexer,
		info_url = :info_url,
		publish_date = :publish_date,
		score = :score,
		created_at = :created_at,
		stalled_since = 0,
		last_amount_left = -1,
		is_manual = :is_manual,
		previous_hash = :previous_hash,
		previous_title = :previous_title,
		previous_indexer = :previous_indexer,
		previous_info_url = :previous_info_url,
		previous_publish_date = :previous_publish_date,
		previous_score = :previous_score
	`
	result, err := s.db.NamedExec(query, t)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	if id == 0 {
		var existing model.EpisodeTorrent
		if err := s.db.Get(&existing, "SELECT * FROM episode_torrents WHERE episode_id = ? LIMIT 1", t.EpisodeId); err != nil {
			return 0, err
		}
		id = existing.ID
	}

	return id, nil
}

func (s *EpisodeTorrentRepository) UpsertTx(tx *sqlx.Tx, t model.EpisodeTorrent) (int64, error) {
	query := `
	INSERT INTO episode_torrents (
		episode_id,
		hash,
		title,
		indexer,
		info_url,
		publish_date,
		score,
		created_at,
		is_manual,
		previous_hash,
		previous_title,
		previous_indexer,
		previous_info_url,
		previous_publish_date,
		previous_score
	)
	VALUES (
		:episode_id,
		:hash,
		:title,
		:indexer,
		:info_url,
		:publish_date,
		:score,
		:created_at,
		:is_manual,
		:previous_hash,
		:previous_title,
		:previous_indexer,
		:previous_info_url,
		:previous_publish_date,
		:previous_score
	)
	ON CONFLICT(episode_id) DO UPDATE SET
		hash = :hash,
		title = :title,
		indexer = :indexer,
		info_url = :info_url,
		publish_date = :publish_date,
		score = :score,
		created_at = :created_at,
		stalled_since = 0,
		last_amount_left = -1,
		is_manual = :is_manual,
		previous_hash = :previous_hash,
		previous_title = :previous_title,
		previous_indexer = :previous_indexer,
		previous_info_url = :previous_info_url,
		previous_publish_date = :previous_publish_date,
		previous_score = :previous_score
	`
	result, err := tx.NamedExec(query, t)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	if id == 0 {
		var existing model.EpisodeTorrent
		if err := tx.Get(&existing, "SELECT * FROM episode_torrents WHERE episode_id = ? LIMIT 1", t.EpisodeId); err != nil {
			return 0, err
		}
		id = existing.ID
	}

	return id, nil
}

func (s *EpisodeTorrentRepository) GetByEpisodeID(episodeID int64) (model.EpisodeTorrent, error) {
	var t model.EpisodeTorrent
	if err := s.db.Get(&t, "SELECT * FROM episode_torrents WHERE episode_id = ? LIMIT 1", episodeID); err != nil {
		return model.EpisodeTorrent{}, err
	}
	return t, nil
}

func (s *EpisodeTorrentRepository) GetByHash(hash string) (model.EpisodeTorrent, error) {
	var t model.EpisodeTorrent
	if err := s.db.Get(&t, "SELECT * FROM episode_torrents WHERE hash = ? COLLATE NOCASE LIMIT 1", hash); err != nil {
		return model.EpisodeTorrent{}, err
	}
	return t, nil
}

func (s *EpisodeTorrentRepository) ListByEpisodeIDs(episodeIDs []int64) ([]model.EpisodeTorrent, error) {
	if len(episodeIDs) == 0 {
		return []model.EpisodeTorrent{}, nil
	}

	query, args, err := sqlx.In("SELECT * FROM episode_torrents WHERE episode_id IN (?)", episodeIDs)
	if err != nil {
		return []model.EpisodeTorrent{}, err
	}

	query = s.db.Rebind(query)

	var torrents []model.EpisodeTorrent
	if err := s.db.Select(&torrents, query, args...); err != nil {
		return []model.EpisodeTorrent{}, err
	}

	return torrents, nil
}

func (s *EpisodeTorrentRepository) UpdateStallState(episodeID int64, stalledSince, lastAmountLeft int64) error {
	_, err := s.db.Exec(
		"UPDATE episode_torrents SET stalled_since = ?, last_amount_left = ? WHERE episode_id = ?",
		stalledSince,
		lastAmountLeft,
		episodeID,
	)
	return err
}

func (s *EpisodeTorrentRepository) ClearPreviousTx(tx *sqlx.Tx, episodeID int64) error {
	_, err := tx.Exec(`
		UPDATE episode_torrents
		SET previous_hash = '',
		    previous_title = '',
		    previous_indexer = '',
		    previous_info_url = '',
		    previous_publish_date = '',
		    previous_score = 0
		WHERE episode_id = ?
	`, episodeID)
	return err
}

func (s *EpisodeTorrentRepository) ClearPrevious(episodeID int64) error {
	_, err := s.db.Exec(`
		UPDATE episode_torrents
		SET previous_hash = '',
		    previous_title = '',
		    previous_indexer = '',
		    previous_info_url = '',
		    previous_publish_date = '',
		    previous_score = 0
		WHERE episode_id = ?
	`, episodeID)
	return err
}

func (s *EpisodeTorrentRepository) RestorePrevious(episodeID int64) (model.EpisodeTorrent, error) {
	_, err := s.db.Exec(`
		UPDATE episode_torrents
		SET hash = previous_hash,
		    title = previous_title,
		    indexer = previous_indexer,
		    info_url = previous_info_url,
		    publish_date = previous_publish_date,
		    score = previous_score,
		    stalled_since = 0,
		    last_amount_left = -1,
		    is_manual = 0,
		    previous_hash = '',
		    previous_title = '',
		    previous_indexer = '',
		    previous_info_url = '',
		    previous_publish_date = '',
		    previous_score = 0
		WHERE episode_id = ? AND previous_hash != ''
	`, episodeID)
	if err != nil {
		return model.EpisodeTorrent{}, err
	}
	return s.GetByEpisodeID(episodeID)
}

func (s *EpisodeTorrentRepository) DeleteByEpisodeID(episodeID int64) error {
	if _, err := s.db.Exec("DELETE FROM episode_torrents WHERE episode_id = ?", episodeID); err != nil {
		return err
	}
	return nil
}

func (s *EpisodeTorrentRepository) DeleteByEpisodeIDs(episodeIDs ...int64) error {
	if len(episodeIDs) <= 0 {
		return nil
	}

	query, args, err := sqlx.In("DELETE FROM episode_torrents WHERE episode_id IN (?)", episodeIDs)
	if err != nil {
		return err
	}

	query = s.db.Rebind(query)

	if _, err := s.db.Exec(query, args...); err != nil {
		return err
	}

	return nil
}

var ErrEpisodeTorrentNotFound = errors.New("episode torrent not found")

var _ EpisodeTorrentRepositoryInterface = (*EpisodeTorrentRepository)(nil)

type EpisodeTorrentRepositoryInterface interface {
	Upsert(t model.EpisodeTorrent) (int64, error)
	UpsertTx(tx *sqlx.Tx, t model.EpisodeTorrent) (int64, error)
	UpdateStallState(episodeID int64, stalledSince, lastAmountLeft int64) error
	ClearPrevious(episodeID int64) error
	ClearPreviousTx(tx *sqlx.Tx, episodeID int64) error
	RestorePrevious(episodeID int64) (model.EpisodeTorrent, error)
	GetByEpisodeID(episodeID int64) (model.EpisodeTorrent, error)
	GetByHash(hash string) (model.EpisodeTorrent, error)
	ListByEpisodeIDs(episodeIDs []int64) ([]model.EpisodeTorrent, error)
	DeleteByEpisodeID(episodeID int64) error
	DeleteByEpisodeIDs(episodeIDs ...int64) error
}
