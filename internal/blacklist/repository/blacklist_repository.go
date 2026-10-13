package repository

import (
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/internal/blacklist/model"
	"github.com/jusoaresg/gorgon/utils"
)

type BlacklistRepositoryInterface interface {
	Create(entry model.ReleaseBlacklist) (int64, error)
	ListActiveByShowID(showID int64, now int64) ([]model.ReleaseBlacklist, error)
	DeleteByID(id int64) error
	DeleteByHashOrTitle(showID int64, infoHash, releaseTitle string) error
}

type BlacklistRepository struct {
	db *sqlx.DB
}

func NewBlacklistRepository(db *sqlx.DB) *BlacklistRepository {
	return &BlacklistRepository{
		db: db,
	}
}

func (r *BlacklistRepository) Create(entry model.ReleaseBlacklist) (int64, error) {
	if entry.CreatedAt == 0 {
		entry.CreatedAt = time.Now().Unix()
	}
	entry.InfoHash = strings.TrimSpace(entry.InfoHash)
	entry.ReleaseTitle = strings.TrimSpace(entry.ReleaseTitle)

	query := `
	INSERT INTO release_blacklist (
		show_id,
		episode_id,
		info_hash,
		release_title,
		indexer,
		reason,
		created_at,
		expires_at
	)
	VALUES (
		:show_id,
		:episode_id,
		:info_hash,
		:release_title,
		:indexer,
		:reason,
		:created_at,
		:expires_at
	)
	`
	result, err := r.db.NamedExec(query, entry)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *BlacklistRepository) ListActiveByShowID(showID int64, now int64) ([]model.ReleaseBlacklist, error) {
	var entries []model.ReleaseBlacklist
	query := `
	SELECT * FROM release_blacklist
	WHERE show_id = ?
	  AND (expires_at IS NULL OR expires_at = 0 OR expires_at > ?)
	ORDER BY created_at DESC
	`
	if err := r.db.Select(&entries, query, showID, now); err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *BlacklistRepository) DeleteByID(id int64) error {
	_, err := r.db.Exec("DELETE FROM release_blacklist WHERE id = ?", id)
	return err
}

func (r *BlacklistRepository) DeleteByHashOrTitle(showID int64, infoHash, releaseTitle string) error {
	hash := strings.TrimSpace(infoHash)
	title := strings.TrimSpace(releaseTitle)

	if hash != "" {
		if _, err := r.db.Exec(
			"DELETE FROM release_blacklist WHERE show_id = ? AND info_hash != '' AND info_hash = ? COLLATE NOCASE",
			showID,
			hash,
		); err != nil {
			return err
		}
	}

	if title != "" {
		normTarget := utils.NormalizeTitle(title)
		entries, err := r.ListActiveByShowID(showID, 0)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.ReleaseTitle != "" && utils.NormalizeTitle(e.ReleaseTitle) == normTarget {
				_ = r.DeleteByID(e.ID)
			}
		}
	}

	return nil
}

var _ BlacklistRepositoryInterface = (*BlacklistRepository)(nil)
