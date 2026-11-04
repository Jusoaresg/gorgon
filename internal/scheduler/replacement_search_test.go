package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type replacementSearchStub struct {
	calls int
	err   error
}

func (s *replacementSearchStub) ProcessSingleEpisode(int) error { s.calls++; return s.err }
func replacementDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`CREATE TABLE release_deletion_operations(id TEXT PRIMARY KEY,status TEXT)`,
		`CREATE TABLE episodes(id INTEGER PRIMARY KEY,tracking TEXT)`,
		`CREATE TABLE episode_torrents(episode_id INTEGER,hash TEXT)`,
		`CREATE TABLE release_replacement_searches(operation_id TEXT PRIMARY KEY,episode_id INTEGER,status TEXT DEFAULT 'pending',attempts INTEGER DEFAULT 0,next_attempt_at INTEGER DEFAULT 0,last_error TEXT DEFAULT '')`,
		`INSERT INTO episodes VALUES(1,'wanted')`,
		`INSERT INTO release_deletion_operations VALUES('op-1','running')`,
		`INSERT INTO release_replacement_searches(operation_id,episode_id) VALUES('op-1',1)`,
	} {
		_, err = db.Exec(q)
		require.NoError(t, err)
	}
	return db
}
func TestReplacementSearchWaitsForCleanupAndDispatchesOnce(t *testing.T) {
	db := replacementDB(t)
	search := &replacementSearchStub{}
	now := time.Unix(1000, 0)
	require.NoError(t, ProcessReplacementSearches(context.Background(), db, search, now))
	require.Zero(t, search.calls)
	_, err := db.Exec("UPDATE release_deletion_operations SET status='completed'")
	require.NoError(t, err)
	require.NoError(t, ProcessReplacementSearches(context.Background(), db, search, now))
	require.Equal(t, 1, search.calls)
	require.NoError(t, ProcessReplacementSearches(context.Background(), db, search, now.Add(time.Hour)))
	require.Equal(t, 1, search.calls)
}

func TestReplacementSearchRetriesFailureAndReconcilesLostAcknowledgment(t *testing.T) {
	db := replacementDB(t)
	search := &replacementSearchStub{err: errors.New("indexer unavailable")}
	now := time.Unix(1000, 0)
	_, err := db.Exec("UPDATE release_deletion_operations SET status='completed'")
	require.NoError(t, err)
	require.NoError(t, ProcessReplacementSearches(context.Background(), db, search, now))
	require.Equal(t, 1, search.calls)
	require.NoError(t, ProcessReplacementSearches(context.Background(), db, search, now))
	require.Equal(t, 1, search.calls)
	search.err = nil
	require.NoError(t, ProcessReplacementSearches(context.Background(), db, search, now.Add(31*time.Second)))
	require.Equal(t, 2, search.calls)
	// Simulate a crash after acquisition but before the handoff acknowledgment.
	_, err = db.Exec("UPDATE release_replacement_searches SET status='running',next_attempt_at=0")
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO episode_torrents VALUES(1,'replacement')")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE episodes SET tracking='snatched'")
	require.NoError(t, err)
	require.NoError(t, ProcessReplacementSearches(context.Background(), db, search, now.Add(time.Hour)))
	require.Equal(t, 2, search.calls)
	var status string
	require.NoError(t, db.Get(&status, "SELECT status FROM release_replacement_searches"))
	require.Equal(t, "completed", status)
}

func TestReplacementSearchRecoversExpiredLeaseAfterRestart(t *testing.T) {
	db := replacementDB(t)
	search := &replacementSearchStub{}
	now := time.Unix(1000, 0)
	_, err := db.Exec("UPDATE release_deletion_operations SET status='completed'")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE release_replacement_searches SET status='running',next_attempt_at=?", now.Add(time.Minute).Unix())
	require.NoError(t, err)
	require.NoError(t, ProcessReplacementSearches(context.Background(), db, search, now))
	require.Zero(t, search.calls)
	require.NoError(t, ProcessReplacementSearches(context.Background(), db, search, now.Add(2*time.Minute)))
	require.Equal(t, 1, search.calls)
}

func TestReplacementSearchConcurrentDispatchDoesNotDuplicateWork(t *testing.T) {
	db := replacementDB(t)
	search := &replacementSearchStub{}
	now := time.Unix(1000, 0)
	_, err := db.Exec("UPDATE release_deletion_operations SET status='completed'")
	require.NoError(t, err)
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		wg.Go(func() { errors <- ProcessReplacementSearches(context.Background(), db, search, now) })
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	require.Equal(t, 1, search.calls)
}
