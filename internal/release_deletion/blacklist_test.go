package release_deletion

import (
	"context"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestBlacklistPersistsBeforeCleanupAndQueuesSearchOnlyAfterCompletion(t *testing.T) {
	s, db, source, link, _ := fixture(t)
	setupBlacklistTables(t, s)
	op, err := s.Delete(context.Background(), Request{EpisodeID: 1, Intention: "blacklist"})
	require.Error(t, err)
	require.FileExists(t, source)
	require.FileExists(t, link)
	var count int
	require.NoError(t, db.Get(&count, "SELECT count(*) FROM release_blacklist WHERE info_hash='abc'"))
	require.Equal(t, 1, count)
	require.NoError(t, db.Get(&count, "SELECT count(*) FROM release_replacement_searches"))
	require.Zero(t, count)
	require.NoError(t, os.Remove(source))
	op, err = s.Resume(context.Background(), op.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", op.Status)
	require.NoError(t, db.Get(&count, "SELECT count(*) FROM release_replacement_searches WHERE operation_id=?", op.ID))
	require.Equal(t, 1, count)
	_, err = s.Delete(context.Background(), Request{EpisodeID: 1, Intention: "blacklist"})
	require.NoError(t, err)
	require.NoError(t, db.Get(&count, "SELECT count(*) FROM release_blacklist"))
	require.Equal(t, 1, count)
	require.NoError(t, db.Get(&count, "SELECT count(*) FROM release_replacement_searches"))
	require.Equal(t, 1, count)
}
func setupBlacklistTables(t *testing.T, s *Service) {
	t.Helper()
	for _, q := range []string{
		`CREATE TABLE release_blacklist(id INTEGER PRIMARY KEY,show_id INTEGER,episode_id INTEGER,info_hash TEXT,release_title TEXT,indexer TEXT,reason TEXT,created_at INTEGER,expires_at INTEGER)`,
		`ALTER TABLE episode_torrents ADD COLUMN title TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE episode_torrents ADD COLUMN indexer TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE release_replacement_searches(operation_id TEXT PRIMARY KEY,episode_id INTEGER NOT NULL,status TEXT NOT NULL DEFAULT 'pending',attempts INTEGER NOT NULL DEFAULT 0,next_attempt_at INTEGER NOT NULL DEFAULT 0,last_error TEXT NOT NULL DEFAULT '')`,
	} {
		_, err := s.DB.Exec(q)
		require.NoError(t, err)
	}
}

func TestBlacklistPersistenceFailurePreventsEveryDestructiveEffect(t *testing.T) {
	s, db, source, link, removeFiles := fixture(t)
	setupBlacklistTables(t, s)
	*removeFiles = true
	_, err := db.Exec(`CREATE TRIGGER reject_blacklist BEFORE INSERT ON release_blacklist BEGIN SELECT RAISE(ABORT,'blacklist unavailable'); END`)
	require.NoError(t, err)
	_, err = s.Delete(context.Background(), Request{EpisodeID: 1, Intention: "blacklist"})
	require.Error(t, err)
	require.FileExists(t, source)
	require.FileExists(t, link)
	var count int
	require.NoError(t, db.Get(&count, "SELECT count(*) FROM release_deletion_operations"))
	require.Zero(t, count)
	var tracking string
	require.NoError(t, db.Get(&tracking, "SELECT tracking FROM episodes WHERE id=1"))
	require.Equal(t, "downloaded", tracking)
}

func TestBlacklistFinalizationAndHandoffRecoverTogether(t *testing.T) {
	s, db, source, _, removeFiles := fixture(t)
	setupBlacklistTables(t, s)
	*removeFiles = true
	_, err := db.Exec(`CREATE TRIGGER reject_handoff BEFORE INSERT ON release_replacement_searches BEGIN SELECT RAISE(ABORT,'handoff unavailable'); END`)
	require.NoError(t, err)
	op, err := s.Delete(context.Background(), Request{EpisodeID: 1, Intention: "blacklist"})
	require.Error(t, err)
	require.Equal(t, "finalizing", op.Stage)
	require.NoFileExists(t, source)
	var count int
	require.NoError(t, db.Get(&count, "SELECT count(*) FROM episode_torrents WHERE episode_id=1"))
	require.Equal(t, 1, count)
	require.NoError(t, db.Get(&count, "SELECT count(*) FROM release_replacement_searches"))
	require.Zero(t, count)
	_, err = db.Exec("DROP TRIGGER reject_handoff")
	require.NoError(t, err)
	restarted := NewWithOptions(db, s.Logger, s.Options)
	op, err = restarted.Resume(context.Background(), op.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", op.Status)
	require.NoError(t, db.Get(&count, "SELECT count(*) FROM release_replacement_searches WHERE operation_id=?", op.ID))
	require.Equal(t, 1, count)
	_, err = restarted.Resume(context.Background(), op.ID)
	require.NoError(t, err)
	require.NoError(t, db.Get(&count, "SELECT count(*) FROM release_replacement_searches"))
	require.Equal(t, 1, count)
}

func TestBlacklistPreservesStallReasonAndExpiry(t *testing.T) {
	s, db, _, _, _ := fixture(t)
	setupBlacklistTables(t, s)
	expiry := int64(123456)
	_, err := s.Delete(context.Background(), Request{Hash: "abc", Intention: "blacklist", BlacklistReason: "stalled download", BlacklistExpiresAt: &expiry})
	require.Error(t, err) // Deliberate remaining download keeps cleanup incomplete.
	var row struct {
		Reason string `db:"reason"`
		Expiry int64  `db:"expires_at"`
	}
	require.NoError(t, db.Get(&row, "SELECT reason,expires_at FROM release_blacklist"))
	require.Equal(t, "stalled download", row.Reason)
	require.Equal(t, expiry, row.Expiry)
}

func TestBlacklistUnassociatedHashCannotDeleteDownload(t *testing.T) {
	s, db, source, link, removeFiles := fixture(t)
	setupBlacklistTables(t, s)
	*removeFiles = true
	_, err := db.Exec("DELETE FROM episode_torrents")
	require.NoError(t, err)
	_, err = s.Delete(context.Background(), Request{Hash: "abc", Intention: "blacklist"})
	require.ErrorIs(t, err, ErrUnsafe)
	require.FileExists(t, source)
	require.FileExists(t, link)
}
