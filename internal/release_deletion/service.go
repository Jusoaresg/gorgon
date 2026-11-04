// Package release_deletion owns the recoverable physical deletion workflow.
package release_deletion

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	qs "github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	qb "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	em "github.com/jusoaresg/gorgon/internal/episode/model"
	cm "github.com/jusoaresg/gorgon/internal/episode_content/model"
	"github.com/jusoaresg/gorgon/utils"
)

var ErrConflict = errors.New("deletion conflicts with an existing operation")
var ErrUnsafe = errors.New("deletion target ownership or filesystem mapping cannot be verified")

type Client interface {
	DeleteTorrent(string, bool) error
	CheckTorrentsWithHash(string, string, *[]qs.CheckTorrentResponse) error
	GetContent(string) ([]cm.EpisodeContent, error)
}
type Request struct {
	EpisodeID          int64  `json:"episode_id"`
	Hash               string `json:"hash"`
	Intention          string `json:"intention"`
	BlacklistReason    string `json:"blacklist_reason,omitempty"`
	BlacklistExpiresAt *int64 `json:"blacklist_expires_at,omitempty"`
}
type Manifest struct {
	LinkIdentities map[string]string `json:"link_identities"`
	Files          []string          `json:"files"`
	Links          []string          `json:"links"`
	Directories    []string          `json:"directories"`
	DownloadRoot   string            `json:"download_root"`
	ShowsRoot      string            `json:"shows_root"`
}
type Operation struct {
	NextAttemptAt int64  `db:"next_attempt_at" json:"next_attempt_at"`
	RetryCount    int    `db:"retry_count" json:"retry_count"`
	Retryable     bool   `db:"retryable" json:"retryable"`
	ID            string `db:"id" json:"id"`
	Target        string `db:"target" json:"target"`
	EpisodeID     int64  `db:"episode_id" json:"episode_id"`
	Hash          string `db:"hash" json:"hash"`
	Intention     string `db:"intention" json:"intention"`
	Status        string `db:"status" json:"status"`
	Stage         string `db:"stage" json:"stage"`
	ManifestJSON  string `db:"manifest" json:"-"`
	Attempts      int    `db:"attempts" json:"attempts"`
	LastError     string `db:"last_error" json:"last_error"`
	CreatedAt     int64  `db:"created_at" json:"created_at"`
	UpdatedAt     int64  `db:"updated_at" json:"updated_at"`
}
type Options struct {
	Now                 func() time.Time
	OnChange            func(Operation)
	Client              Client
	DownloadRoot        string
	ShowsRoot           string
	ConfirmationTimeout time.Duration
	PollInterval        time.Duration
}
type Service struct {
	DB      *sqlx.DB
	Logger  *slog.Logger
	Options Options
}

var executionMu sync.Mutex

func New(db *sqlx.DB, logger *slog.Logger) (*Service, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}
	client, err := qb.NewQBittorrentService(logger)
	if err != nil {
		return nil, err
	}
	client.APIService.Client.Timeout = 2 * time.Second
	return NewWithOptions(db, logger, Options{Client: client, DownloadRoot: cfg.QBittorrentDownloadFolder, ShowsRoot: cfg.ShowsFolder}), nil
}
func NewWithOptions(db *sqlx.DB, logger *slog.Logger, o Options) *Service {
	if o.ConfirmationTimeout <= 0 {
		o.ConfirmationTimeout = 2 * time.Second
	}
	if o.PollInterval <= 0 {
		o.PollInterval = 100 * time.Millisecond
	}
	return &Service{db, logger, o}
}
func (s *Service) Get(id string) (Operation, error) {
	var op Operation
	err := s.DB.Get(&op, "SELECT * FROM release_deletion_operations WHERE id = ?", id)
	return op, err
}
func (s *Service) Delete(ctx context.Context, r Request) (Operation, error) {
	executionMu.Lock()
	defer executionMu.Unlock()
	op, err := s.prepare(r)
	if err != nil {
		return op, err
	}
	return s.run(ctx, op)
}
func (s *Service) Resume(ctx context.Context, id string) (Operation, error) {
	executionMu.Lock()
	defer executionMu.Unlock()
	op, err := s.Get(id)
	if err != nil {
		return op, err
	}
	return s.run(ctx, op)
}
func ownedPath(root, name string) (string, error) {
	if root == "" || name == "" || !filepath.IsAbs(root) {
		return "", ErrUnsafe
	}
	root = filepath.Clean(root)
	p := name
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	p = filepath.Clean(p)
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", ErrUnsafe
	}
	// Never follow a directory symlink, including one above the configured root.
	for d := filepath.Dir(p); ; d = filepath.Dir(d) {
		info, e := os.Lstat(d)
		if e != nil && !os.IsNotExist(e) {
			return "", e
		}
		if e == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return "", ErrUnsafe
		}
		if d == filepath.Dir(d) {
			break
		}
	}
	return p, nil
}
func (s *Service) prepare(r Request) (Operation, error) {
	var op Operation
	if r.Intention != "skipped" && r.Intention != "wanted" && r.Intention != "missing" && r.Intention != "blacklist" {
		return op, fmt.Errorf("invalid deletion intention")
	}
	if r.EpisodeID == 0 && r.Hash == "" {
		return op, ErrUnsafe
	}
	r.Hash = strings.ToLower(strings.TrimSpace(r.Hash))
	if r.EpisodeID == 0 && r.Hash != "" {
		var ids []int64
		if err := s.DB.Select(&ids, "SELECT DISTINCT episode_id FROM episode_torrents WHERE hash = ? COLLATE NOCASE", r.Hash); err != nil {
			return op, err
		}
		if len(ids) > 1 {
			return op, fmt.Errorf("%w: torrent is associated with other episodes", ErrUnsafe)
		}
		if len(ids) == 1 {
			r.EpisodeID = ids[0]
		} else {
			// Finalization removes associations. Preserve request identity for retries
			// after completion or a lost response, without guessing an episode.
			err := s.DB.Get(&op, "SELECT * FROM release_deletion_operations WHERE target = ? ORDER BY created_at DESC, rowid DESC LIMIT 1", "hash:"+r.Hash)
			if err == nil {
				if op.Intention != r.Intention {
					return op, ErrConflict
				}
				if op.Status != "completed" {
					return op, nil
				}
				var torrents []qs.CheckTorrentResponse
				if err := s.Options.Client.CheckTorrentsWithHash("", r.Hash, &torrents); err != nil {
					return op, err
				}
				if len(torrents) == 0 {
					return op, nil
				}
			} else if !errors.Is(err, sql.ErrNoRows) {
				return op, err
			}
		}
	}
	if r.Intention == "blacklist" && r.EpisodeID == 0 {
		return op, fmt.Errorf("%w: blacklisting requires a tracked episode", ErrUnsafe)
	}
	var ep em.Episode
	if r.EpisodeID != 0 {
		if err := s.DB.Get(&ep, "SELECT * FROM episodes WHERE id = ?", r.EpisodeID); err != nil {
			return op, err
		}
		var hash string
		err := s.DB.Get(&hash, "SELECT hash FROM episode_torrents WHERE episode_id = ?", r.EpisodeID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				previousErr := s.DB.Get(&op, "SELECT * FROM release_deletion_operations WHERE episode_id = ? AND intention = ? ORDER BY created_at DESC LIMIT 1", r.EpisodeID, r.Intention)
				if previousErr == nil {
					return op, nil
				}
			}
			return op, fmt.Errorf("%w: missing episode torrent", ErrUnsafe)
		}
		if r.Hash != "" && !strings.EqualFold(r.Hash, hash) {
			return op, ErrConflict
		}
		r.Hash = hash
	}
	r.Hash = strings.ToLower(strings.TrimSpace(r.Hash))
	if r.Hash == "" {
		return op, ErrUnsafe
	}
	target := "hash:" + r.Hash
	err := s.DB.Get(&op, "SELECT * FROM release_deletion_operations WHERE target = ? AND status != 'completed'", target)
	if err == nil {
		if op.Intention != r.Intention || op.EpisodeID != r.EpisodeID {
			return op, ErrConflict
		}
		return op, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return op, err
	}
	var ids []int64
	if err = s.DB.Select(&ids, "SELECT episode_id FROM episode_torrents WHERE hash = ? COLLATE NOCASE", r.Hash); err != nil {
		return op, err
	}
	// An episode request must never delete a season pack shared by other episodes.
	if len(ids) > 1 || (len(ids) == 1 && r.EpisodeID == 0) {
		return op, fmt.Errorf("%w: torrent is associated with other episodes", ErrUnsafe)
	}
	var torrents []qs.CheckTorrentResponse
	if err = s.Options.Client.CheckTorrentsWithHash("", r.Hash, &torrents); err != nil {
		return op, err
	}
	if len(torrents) != 1 || !strings.EqualFold(torrents[0].Hash, r.Hash) {
		return op, fmt.Errorf("%w: torrent metadata unavailable", ErrUnsafe)
	}
	torrent := torrents[0]
	save := filepath.Clean(torrent.SavePath)
	root := filepath.Clean(s.Options.DownloadRoot)
	if save != root {
		if _, err = ownedPath(root, save); err != nil {
			return op, fmt.Errorf("%w: client save path differs from local download mapping", ErrUnsafe)
		}
	}
	files, err := s.Options.Client.GetContent(r.Hash)
	if err != nil {
		return op, err
	}
	if len(files) == 0 {
		return op, ErrUnsafe
	}
	m := Manifest{DownloadRoot: root, ShowsRoot: s.Options.ShowsRoot, LinkIdentities: map[string]string{}}
	for _, f := range files {
		if filepath.IsAbs(f.Name) {
			return op, ErrUnsafe
		}
		p, e := ownedPath(root, filepath.Join(save, f.Name))
		if e != nil {
			return op, e
		}
		if info, e := os.Lstat(p); e == nil && (info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return op, ErrUnsafe
		} else if e != nil && !os.IsNotExist(e) {
			return op, e
		}
		m.Files = append(m.Files, p)
	}
	if err = s.rejectSharedSources(r.EpisodeID, root, m.Files); err != nil {
		return op, err
	}
	// A content directory is verified as a whole, including extra files. It is never recursively removed locally.
	contentPath := torrent.ContentPath
	if contentPath != "" {
		p, e := ownedPath(root, contentPath)
		if e != nil {
			return op, e
		}
		if info, e := os.Lstat(p); e == nil && info.IsDir() {
			m.Directories = append(m.Directories, p)
		} else if e != nil && !os.IsNotExist(e) {
			return op, e
		}
	}
	if r.EpisodeID != 0 {
		var contents []cm.EpisodeContent
		if err = s.DB.Select(&contents, "SELECT * FROM episode_contents WHERE episode_id = ?", r.EpisodeID); err != nil {
			return op, err
		}
		var showName string
		if err = s.DB.Get(&showName, "SELECT name FROM shows WHERE id = ?", ep.ShowID); err != nil {
			return op, err
		}
		for _, c := range contents {
			p := c.FilePath
			if p == "" {
				p, err = utils.SymlinkPathForEpisode(s.Options.ShowsRoot, showName, ep, c)
				if err != nil {
					return op, err
				}
			}
			p, err = ownedPath(s.Options.ShowsRoot, p)
			if err != nil {
				return op, err
			}
			var count int
			if err = s.DB.Get(&count, "SELECT count(*) FROM episode_contents WHERE file_path = ? AND episode_id != ?", p, r.EpisodeID); err != nil {
				return op, err
			}
			if count > 0 {
				return op, ErrUnsafe
			}
			if info, e := os.Lstat(p); e == nil {
				if info.IsDir() {
					return op, ErrUnsafe
				}
				owned := false
				for _, source := range m.Files {
					if info.Mode()&os.ModeSymlink != 0 {
						target, e := os.Readlink(p)
						if e != nil {
							return op, e
						}
						if !filepath.IsAbs(target) {
							target = filepath.Join(filepath.Dir(p), target)
						}
						owned = owned || filepath.Clean(target) == source
					} else {
						src, e := os.Stat(source)
						owned = owned || (e == nil && os.SameFile(info, src))
					}
				}
				if !owned {
					return op, ErrUnsafe
				}
			} else if !os.IsNotExist(e) {
				return op, e
			}
			if info, e := os.Lstat(p); e == nil {
				m.LinkIdentities[p] = fileIdentity(p, info)
			}
			m.Links = append(m.Links, p)
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return op, err
	}
	now := s.now().Unix()
	op = Operation{ID: uuid.NewString(), Target: target, EpisodeID: r.EpisodeID, Hash: r.Hash, Intention: r.Intention, Status: "pending", Stage: "prepared", ManifestJSON: string(b), CreatedAt: now, UpdatedAt: now}
	query := `INSERT INTO release_deletion_operations (id,target,episode_id,hash,intention,status,stage,manifest,attempts,last_error,created_at,updated_at) VALUES (:id,:target,:episode_id,:hash,:intention,:status,:stage,:manifest,:attempts,:last_error,:created_at,:updated_at)`
	if r.Intention != "blacklist" {
		_, err = s.DB.NamedExec(query, op)
		return op, err
	}
	tx, err := s.DB.Beginx()
	if err != nil {
		return op, err
	}
	defer tx.Rollback()
	if _, err = tx.NamedExec(query, op); err != nil {
		return Operation{}, err
	}
	reason := r.BlacklistReason
	if reason == "" {
		reason = "manual blacklist"
	}
	result, err := tx.Exec(`INSERT INTO release_blacklist(show_id,episode_id,info_hash,release_title,indexer,reason,created_at,expires_at) SELECT ?,episode_id,hash,title,indexer,?,?,? FROM episode_torrents WHERE episode_id=? AND hash=? COLLATE NOCASE`, ep.ShowID, reason, now, r.BlacklistExpiresAt, r.EpisodeID, r.Hash)
	if err != nil {
		return Operation{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return Operation{}, err
	}
	if count != 1 {
		return Operation{}, ErrConflict
	}
	if err = tx.Commit(); err != nil {
		return Operation{}, err
	}
	return op, nil
}
func (s *Service) checkpoint(op *Operation, status, stage string, err error) error {
	op.Status = status
	op.Stage = stage
	op.LastError = ""
	if err != nil {
		op.LastError = "Release deletion is incomplete; check the torrent client, filesystem permissions, and server logs."
		if s.Logger != nil {
			s.Logger.Error("release deletion failed", "operation_id", op.ID, "stage", stage, "error", err)
		}
	}
	op.UpdatedAt = s.now().Unix()
	op.NextAttemptAt = 0
	if err != nil {
		op.Retryable = retryable(err)
		if op.Retryable && op.RetryCount < MaxAutomaticAttempts {
			index := op.RetryCount - 1
			if index < 0 {
				index = 0
			}
			op.NextAttemptAt = s.now().Add(retryDelays[index]).Unix()
		}
	}
	_, e := s.DB.Exec("UPDATE release_deletion_operations SET status=?,stage=?,attempts=?,last_error=?,updated_at=?,next_attempt_at=?,retry_count=?,retryable=? WHERE id=?", op.Status, op.Stage, op.Attempts, op.LastError, op.UpdatedAt, op.NextAttemptAt, op.RetryCount, op.Retryable, op.ID)
	if e == nil {
		s.notify(*op)
	}
	return e
}

func absent(path string) error {
	_, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("owned path still exists: %s", path)
}
func (s *Service) run(ctx context.Context, op Operation) (Operation, error) {
	if op.Status == "completed" {
		return op, nil
	}
	op.Attempts++
	op.RetryCount++
	fail := func(err error) (Operation, error) {
		e := s.checkpoint(&op, "failed", op.Stage, err)
		return op, errors.Join(err, e)
	}
	var m Manifest
	if err := json.Unmarshal([]byte(op.ManifestJSON), &m); err != nil {
		return fail(err)
	}
	if err := s.checkpoint(&op, "running", op.Stage, nil); err != nil {
		return op, err
	}
	if op.Stage == "prepared" {
		var torrents []qs.CheckTorrentResponse
		if err := s.Options.Client.CheckTorrentsWithHash("", op.Hash, &torrents); err != nil {
			return fail(err)
		}
		if len(torrents) > 0 {
			if err := s.Options.Client.DeleteTorrent(op.Hash, true); err != nil {
				return fail(err)
			}
		}
		if err := s.checkpoint(&op, "running", "confirming", nil); err != nil {
			return op, err
		}
	}
	if op.Stage == "confirming" {
		deadline := time.Now().Add(s.Options.ConfirmationTimeout)
		for {
			var torrents []qs.CheckTorrentResponse
			err := s.Options.Client.CheckTorrentsWithHash("", op.Hash, &torrents)
			if err != nil {
				return fail(err)
			}
			if len(torrents) > 0 {
				err = errors.New("torrent still exists")
			}
			for _, p := range append(append([]string{}, m.Files...), m.Directories...) {
				if _, e := ownedPath(m.DownloadRoot, p); e != nil {
					return fail(e)
				}
				err = errors.Join(err, absent(p))
			}
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				return fail(err)
			}
			select {
			case <-ctx.Done():
				return fail(ctx.Err())
			case <-time.After(s.Options.PollInterval):
			}
		}
		if err := s.checkpoint(&op, "running", "links", nil); err != nil {
			return op, err
		}
	}
	if op.Stage == "links" {
		for _, p := range m.Links {
			if _, err := ownedPath(m.ShowsRoot, p); err != nil {
				return fail(err)
			}
			if info, err := os.Lstat(p); err == nil && info.IsDir() {
				return fail(ErrUnsafe)
			} else if err != nil && !os.IsNotExist(err) {
				return fail(err)
			}
			if info, e := os.Lstat(p); e == nil && (m.LinkIdentities[p] == "" || fileIdentity(p, info) != m.LinkIdentities[p]) {
				return fail(ErrUnsafe)
			}
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return fail(err)
			}
		}
		if err := s.checkpoint(&op, "running", "finalizing", nil); err != nil {
			return op, err
		}
	}
	if op.Stage != "finalizing" {
		return fail(fmt.Errorf("unknown deletion stage %q", op.Stage))
	}
	tx, err := s.DB.Beginx()
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback()
	txFail := func(e error) (Operation, error) { _ = tx.Rollback(); return fail(e) }
	if op.EpisodeID != 0 {
		for _, query := range []string{"DELETE FROM episode_contents WHERE episode_id = ?", "DELETE FROM episode_torrents WHERE episode_id = ?"} {
			if _, err = tx.Exec(query, op.EpisodeID); err != nil {
				return txFail(err)
			}
		}
		tracking := op.Intention
		if tracking == "blacklist" {
			tracking = "wanted"
			if _, err = tx.Exec("INSERT INTO release_replacement_searches(operation_id,episode_id) VALUES(?,?) ON CONFLICT(operation_id) DO NOTHING", op.ID, op.EpisodeID); err != nil {
				return txFail(err)
			}
		}
		if _, err = tx.Exec("UPDATE episodes SET tracking = ? WHERE id = ?", tracking, op.EpisodeID); err != nil {
			return txFail(err)
		}
	}
	if _, err = tx.Exec("UPDATE release_deletion_operations SET status='completed',stage='completed',last_error='',next_attempt_at=0,updated_at=? WHERE id=?", time.Now().Unix(), op.ID); err != nil {
		return txFail(err)
	}
	if err = tx.Commit(); err != nil {
		return fail(err)
	}
	completed, e := s.Get(op.ID)
	if e == nil {
		s.notify(completed)
	}
	return completed, e
}
