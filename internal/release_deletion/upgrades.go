package release_deletion

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	qs "github.com/jusoaresg/gorgon/external/qbittorrent/schema"
)

// PrepareUpgradeSnapshotWhileGuarded persists the old release before its torrent
// association changes. The caller must hold release_guard.Lock. Deferred snapshots
// never authorize deletion until the replacement is complete and locally verified.
func (s *Service) PrepareUpgradeSnapshotWhileGuarded(ctx context.Context, episodeID int64, candidateHash string) (Operation, error) {
	if err := ctx.Err(); err != nil {
		return Operation{}, err
	}
	candidateHash = strings.ToLower(strings.TrimSpace(candidateHash))
	if candidateHash == "" {
		return Operation{}, ErrUnsafe
	}
	var oldHash string
	if err := s.DB.Get(&oldHash, "SELECT hash FROM episode_torrents WHERE episode_id=?", episodeID); err != nil {
		return Operation{}, err
	}
	if strings.EqualFold(oldHash, candidateHash) {
		return Operation{}, ErrConflict
	}
	op, err := s.prepare(Request{EpisodeID: episodeID, Intention: "upgrade", candidateHash: candidateHash})
	if err != nil {
		return op, err
	}
	var m Manifest
	if err = json.Unmarshal([]byte(op.ManifestJSON), &m); err != nil {
		return op, err
	}
	if m.CandidateHash != candidateHash {
		return op, ErrConflict
	}
	return op, nil
}

// CompleteUpgradeSnapshotWhileGuarded releases deferred cleanup only after the
// candidate is ready. Retrying reuses the durable old manifest, never the new hash.
func (s *Service) CompleteUpgradeSnapshotWhileGuarded(ctx context.Context, episodeID int64, candidateHash string) (Operation, error) {
	var op Operation
	err := s.DB.Get(&op, "SELECT * FROM release_deletion_operations WHERE episode_id=? AND intention='upgrade' ORDER BY rowid DESC LIMIT 1", episodeID)
	if err != nil {
		return op, err
	}
	var m Manifest
	if err = json.Unmarshal([]byte(op.ManifestJSON), &m); err != nil {
		return op, err
	}
	if !strings.EqualFold(m.CandidateHash, candidateHash) {
		return op, ErrConflict
	}
	if op.Status == "completed" {
		return op, nil
	}
	if err = s.verifyUpgradeCandidate(op, m); err != nil {
		return op, err
	}
	if op.Stage == "deferred" {
		if err = s.checkpoint(&op, "pending", "prepared", nil); err != nil {
			return op, err
		}
	}
	return s.run(ctx, op)
}

// AbortUpgradeSnapshotWhileGuarded only cancels an unstarted snapshot whose old
// association is still intact, as after a failed snatch transaction.
func (s *Service) AbortUpgradeSnapshotWhileGuarded(op Operation) error {
	_, err := s.DB.Exec(`UPDATE release_deletion_operations SET status='completed',stage='completed' WHERE id=? AND stage='deferred' AND EXISTS (SELECT 1 FROM episode_torrents WHERE episode_id=? AND hash=? COLLATE NOCASE)`, op.ID, op.EpisodeID, op.Hash)
	return err
}

func (s *Service) verifyUpgradeCandidate(op Operation, m Manifest) error {
	var hash string
	if err := s.DB.Get(&hash, "SELECT hash FROM episode_torrents WHERE episode_id=?", op.EpisodeID); err != nil {
		return err
	}
	if m.CandidateHash == "" || !strings.EqualFold(hash, m.CandidateHash) {
		return ErrConflict
	}
	var torrents []qs.CheckTorrentResponse
	if err := s.Options.Client.CheckTorrentsWithHash("", hash, &torrents); err != nil {
		return err
	}
	if len(torrents) != 1 || !strings.EqualFold(torrents[0].Hash, hash) || torrents[0].Progress < 1 {
		return fmt.Errorf("replacement download is not ready")
	}
	contents, err := s.Options.Client.GetContent(hash)
	if err != nil {
		return err
	}
	if len(contents) == 0 {
		return ErrUnsafe
	}
	for _, content := range contents {
		if filepath.IsAbs(content.Name) {
			return ErrUnsafe
		}
		p, err := ownedPath(m.DownloadRoot, filepath.Join(torrents[0].SavePath, content.Name))
		if err != nil {
			return err
		}
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return ErrUnsafe
		}
		for _, old := range m.Files {
			if p == old {
				return ErrUnsafe
			}
			oldInfo, e := os.Stat(old)
			if e == nil && os.SameFile(info, oldInfo) {
				return ErrUnsafe
			}
			if e != nil && !errors.Is(e, os.ErrNotExist) {
				return e
			}
		}
		for _, dir := range m.Directories {
			rel, e := filepath.Rel(dir, p)
			if e != nil || rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
				return ErrUnsafe
			}
		}
	}
	return nil
}

// HasUpgradeSnapshotWhileGuarded identifies the import that may finish its own
// protected replacement; all unrelated mutation callers retain the normal guard.
func (s *Service) HasUpgradeSnapshotWhileGuarded(episodeID int64, hash string) (bool, error) {
	var manifest string
	err := s.DB.Get(&manifest, `SELECT manifest FROM release_deletion_operations WHERE episode_id=? AND intention='upgrade' ORDER BY rowid DESC LIMIT 1`, episodeID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var m Manifest
	if err = json.Unmarshal([]byte(manifest), &m); err != nil {
		return false, err
	}
	return strings.EqualFold(m.CandidateHash, hash), nil
}
