package release_deletion

import (
	"context"
	"errors"
	qb "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	"github.com/jusoaresg/gorgon/internal/release_guard"
	"github.com/stretchr/testify/require"
	"net/http"
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

func TestAcceptedSubmissionSurvivesRequestCancellationAndWorkerRestart(t *testing.T) {
	s, _, _, _, deleteFiles := fixture(t)
	*deleteFiles = true
	requestContext, cancel := context.WithCancel(context.Background())
	op, err := s.Submit(requestContext, Request{EpisodeID: 1, Intention: "skipped"})
	require.NoError(t, err)
	cancel()
	restarted := NewWithOptions(s.DB, s.Logger, s.Options)
	require.NoError(t, restarted.RecoverDue(context.Background(), time.Now()))
	result, err := restarted.Get(op.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", result.Status)
	require.Equal(t, op.ID, result.ID)
}

func TestRetryDuringQueuedHydrationCannotBeOverwrittenByWorker(t *testing.T) {
	s, db, _, _, _ := fixture(t)
	op, err := s.Submit(context.Background(), Request{EpisodeID: 1, Intention: "wanted"})
	require.NoError(t, err)
	_, err = db.Exec("UPDATE release_deletion_operations SET status='failed',retry_count=4 WHERE id=?", op.ID)
	require.NoError(t, err)
	entered, release := make(chan struct{}), make(chan struct{})
	client := s.Options.Client.(*qb.QBittorrentService)
	client.APIService.Client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/v2/torrents/info" {
			close(entered)
			<-release
			return nil, errors.New("metadata unavailable")
		}
		return http.DefaultTransport.RoundTrip(r)
	})
	done := make(chan struct{})
	go func() { defer close(done); _, _ = s.Resume(context.Background(), op.ID) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not start metadata request")
	}
	_, retryErr := s.SubmitRetry(context.Background(), op.ID)
	close(release)
	<-done
	require.ErrorIs(t, retryErr, release_guard.ErrBusy)
	failed, err := s.Get(op.ID)
	require.NoError(t, err)
	require.Equal(t, 5, failed.RetryCount)
	accepted, err := s.SubmitRetry(context.Background(), op.ID)
	require.NoError(t, err)
	require.Equal(t, "pending", accepted.Status)
	require.Zero(t, accepted.RetryCount)
	require.Greater(t, accepted.Version, failed.Version)
}
