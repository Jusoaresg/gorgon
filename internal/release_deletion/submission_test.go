package release_deletion

import (
	"context"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
	"time"
)

func TestSubmitPersistsWithoutCallingTorrentClient(t *testing.T) {
	s, _, _, _, _ := fixture(t)
	s.Options.Client = nil
	op, err := s.Submit(context.Background(), Request{EpisodeID: 1, Intention: "skipped"})
	require.NoError(t, err)
	require.Equal(t, "queued", op.Stage)
	require.Equal(t, "pending", op.Status)
	stored, err := s.Get(op.ID)
	require.NoError(t, err)
	require.Equal(t, op, stored)
	same, err := s.Submit(context.Background(), Request{EpisodeID: 1, Intention: "skipped"})
	require.NoError(t, err)
	require.Equal(t, op.ID, same.ID)
	_, err = s.Submit(context.Background(), Request{EpisodeID: 1, Intention: "wanted"})
	require.ErrorIs(t, err, ErrConflict)
}
func TestManagedSubmittedOperationHydratesAndCompletes(t *testing.T) {
	s, _, _, _, deleteFiles := fixture(t)
	*deleteFiles = true
	op, err := s.Submit(context.Background(), Request{EpisodeID: 1, Intention: "skipped"})
	require.NoError(t, err)
	require.NoError(t, s.RecoverDue(context.Background(), time.Now()))
	op, err = s.Get(op.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", op.Status)
	require.Greater(t, op.Version, int64(1))
}

func TestHashSubmissionRevalidatesCompletedHistoryRatherThanReportingStaleSuccess(t *testing.T) {
	s, _, _, _, deleteFiles := fixture(t)
	*deleteFiles = true
	first, err := s.Delete(context.Background(), Request{EpisodeID: 1, Intention: "skipped"})
	require.NoError(t, err)
	next, err := s.Submit(context.Background(), Request{Hash: "abc", Intention: "skipped"})
	require.NoError(t, err)
	require.NotEqual(t, first.ID, next.ID)
	require.Equal(t, "pending", next.Status)
	require.NoError(t, s.RecoverDue(context.Background(), time.Now()))
	next, err = s.Get(next.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", next.Status)
}

func TestRelativeConfiguredRootsAreCanonicalAndEmptyRootsRemainUntrusted(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	s := NewWithOptions(nil, nil, Options{DownloadRoot: "downloads", ShowsRoot: "shows"})
	require.Equal(t, filepath.Join(root, "downloads"), s.Options.DownloadRoot)
	require.Equal(t, filepath.Join(root, "shows"), s.Options.ShowsRoot)
	empty := NewWithOptions(nil, nil, Options{})
	require.Empty(t, empty.Options.DownloadRoot)
	require.Empty(t, empty.Options.ShowsRoot)
}

func TestQueuedBlacklistPreservesReasonExpiryAndCapturesBeforeEffects(t *testing.T) {
	s, db, _, _, deleteFiles := fixture(t)
	setupBlacklistTables(t, s)
	*deleteFiles = true
	expiry := int64(123456)
	op, err := s.Submit(context.Background(), Request{Hash: "abc", Intention: "blacklist", BlacklistReason: "stalled download", BlacklistExpiresAt: &expiry})
	require.NoError(t, err)
	require.NoError(t, s.RecoverDue(context.Background(), time.Now()))
	op, err = s.Get(op.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", op.Status)
	var reason string
	var actual int64
	require.NoError(t, db.Get(&reason, "SELECT reason FROM release_blacklist"))
	require.Equal(t, "stalled download", reason)
	require.NoError(t, db.Get(&actual, "SELECT expires_at FROM release_blacklist"))
	require.Equal(t, expiry, actual)
	var count int
	require.NoError(t, db.Get(&count, "SELECT count(*) FROM release_replacement_searches WHERE operation_id=?", op.ID))
	require.Equal(t, 1, count)
}
