// Package release_guard serializes release mutations with deletion checkpoints.
package release_guard

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jmoiron/sqlx"
)

var ErrBusy = errors.New("release mutation is busy; retry shortly")
var ErrProtected = errors.New("release is protected by an incomplete deletion")
var ErrStale = errors.New("release work is stale; reload the episode")

// A conservative process-wide gate also covers aliases of the same SQLite DB
// and whole-show folder migrations. It intentionally trades mutation throughput
// for a single lock order; no guarded caller may recursively acquire the gate.
var gate sync.Mutex

func unlockOnce() func()     { var once sync.Once; return func() { once.Do(gate.Unlock) } }
func Lock(_ *sqlx.DB) func() { gate.Lock(); return unlockOnce() }
func TryLock(_ *sqlx.DB) (func(), bool) {
	if !gate.TryLock() {
		return nil, false
	}
	return unlockOnce(), true
}

// Check must be called while holding the gate, before reading mutable ownership.
// Failed operations remain protected until explicitly recovered and finalized.
func Check(db *sqlx.DB, episodeID int64, hash string) error {
	if db == nil {
		return fmt.Errorf("release database unavailable")
	}
	var count int
	err := db.Get(&count, `SELECT count(*) FROM release_deletion_operations o WHERE o.status!='completed' AND ((?!=0 AND o.episode_id=?) OR (?!='' AND lower(trim(o.hash))=?) OR (?!=0 AND EXISTS (SELECT 1 FROM episode_torrents t WHERE t.episode_id=? AND lower(trim(t.hash))=lower(trim(o.hash)))))`, episodeID, episodeID, strings.TrimSpace(hash), strings.ToLower(strings.TrimSpace(hash)), episodeID, episodeID)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrProtected
	}
	return nil
}

// CheckShow protects whole-folder migrations if any episode or associated hash
// in that show is still owned by an incomplete deletion operation.
func CheckShow(db *sqlx.DB, showID int64) error {
	var count int
	err := db.Get(&count, `SELECT count(*) FROM release_deletion_operations o WHERE o.status!='completed' AND (EXISTS (SELECT 1 FROM episodes e WHERE e.show_id=? AND e.id=o.episode_id) OR EXISTS (SELECT 1 FROM episode_torrents t JOIN episodes e ON e.id=t.episode_id WHERE e.show_id=? AND lower(trim(t.hash))=lower(trim(o.hash))))`, showID, showID)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrProtected
	}
	return nil
}
