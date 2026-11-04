package release_deletion

import (
	"context"
	qb "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecoveryPreservesIntentAndExhaustsAutomaticRetries(t *testing.T) {
	s, db, source, link, _ := fixture(t)
	now := time.Unix(1000, 0)
	s.Options.Now = func() time.Time { return now }
	op, err := s.Delete(context.Background(), Request{EpisodeID: 1, Intention: "wanted"})
	require.Error(t, err)
	require.True(t, op.Retryable)
	require.Equal(t, now.Add(5*time.Second).Unix(), op.NextAttemptAt)
	for i := 0; i < 4; i++ {
		now = time.Unix(op.NextAttemptAt, 0)
		require.NoError(t, s.RecoverDue(context.Background(), now))
		op, err = s.Get(op.ID)
		require.NoError(t, err)
	}
	require.Equal(t, 5, op.RetryCount)
	require.Zero(t, op.NextAttemptAt)
	require.Equal(t, "failed", op.Status)
	require.FileExists(t, link)
	require.NoError(t, os.Remove(source))
	op, err = s.Retry(context.Background(), op.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", op.Status)
	var tracking string
	require.NoError(t, db.Get(&tracking, "SELECT tracking FROM episodes WHERE id=1"))
	require.Equal(t, "wanted", tracking)
}

func TestRecoveryReconcilesEveryPersistedCheckpoint(t *testing.T) {
	for _, stage := range []string{"prepared", "confirming", "links", "finalizing"} {
		t.Run(stage, func(t *testing.T) {
			s, db, source, link, _ := fixture(t)
			op, err := s.Delete(context.Background(), Request{EpisodeID: 1, Intention: "skipped"})
			require.Error(t, err)
			require.NoError(t, os.Remove(source))
			if stage == "finalizing" {
				require.NoError(t, os.Remove(link))
			}
			_, err = db.Exec("UPDATE release_deletion_operations SET stage=?,status='running' WHERE id=?", stage, op.ID)
			require.NoError(t, err)
			require.NoError(t, s.RecoverDue(context.Background(), time.Now()))
			op, err = s.Get(op.ID)
			require.NoError(t, err)
			require.Equal(t, "completed", op.Status)
			_, err = os.Lstat(link)
			require.True(t, os.IsNotExist(err))
		})
	}
}

func TestDatabaseFinalizationFailurePreservesRecoveryAndTracking(t *testing.T) {
	s, db, source, link, _ := fixture(t)
	op, err := s.Delete(context.Background(), Request{EpisodeID: 1, Intention: "wanted"})
	require.Error(t, err)
	require.NoError(t, os.Remove(source))
	_, err = db.Exec(`CREATE TRIGGER reject_finalization BEFORE UPDATE OF tracking ON episodes BEGIN SELECT RAISE(FAIL,'temporary database failure'); END`)
	require.NoError(t, err)
	op, err = s.Resume(context.Background(), op.ID)
	require.Error(t, err)
	require.Equal(t, "finalizing", op.Stage)
	_, err = os.Lstat(link)
	require.True(t, os.IsNotExist(err))
	var tracking string
	require.NoError(t, db.Get(&tracking, "SELECT tracking FROM episodes WHERE id=1"))
	require.Equal(t, "downloaded", tracking)
	var associations int
	require.NoError(t, db.Get(&associations, "SELECT count(*) FROM episode_torrents"))
	require.Equal(t, 1, associations)
	_, err = db.Exec("DROP TRIGGER reject_finalization")
	require.NoError(t, err)
	op, err = s.Retry(context.Background(), op.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", op.Status)
}

func TestReplacedLinkIsActionableAndPreserved(t *testing.T) {
	s, _, source, link, _ := fixture(t)
	op, err := s.Delete(context.Background(), Request{EpisodeID: 1, Intention: "skipped"})
	require.Error(t, err)
	require.NoError(t, os.Remove(source))
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.Mkdir(link, 0700))
	op, err = s.Resume(context.Background(), op.ID)
	require.ErrorIs(t, err, ErrUnsafe)
	require.False(t, op.Retryable)
	require.Zero(t, op.NextAttemptAt)
	info, err := os.Stat(link)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestRetryErrorClassification(t *testing.T) {
	require.False(t, retryable(os.ErrPermission))
	require.False(t, retryable(ErrUnsafe))
	require.False(t, retryable(ErrConflict))
	require.True(t, retryable(context.DeadlineExceeded))
}

func TestTimeoutAfterClientEffectReconcilesWithoutLosingManifest(t *testing.T) {
	s, _, source, link, deleteFiles := fixture(t)
	*deleteFiles = true
	client := s.Options.Client.(*qb.QBittorrentService)
	client.APIService.Client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		resp, err := http.DefaultTransport.RoundTrip(r)
		if err == nil && r.URL.Path == "/api/v2/torrents/delete" {
			resp.Body.Close()
			return nil, context.DeadlineExceeded
		}
		return resp, err
	})
	op, err := s.Delete(context.Background(), Request{EpisodeID: 1, Intention: "skipped"})
	require.Error(t, err)
	require.Equal(t, "prepared", op.Stage)
	_, err = os.Stat(source)
	require.True(t, os.IsNotExist(err))
	require.FileExists(t, link)
	op, err = s.Resume(context.Background(), op.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", op.Status)
	_, err = os.Lstat(link)
	require.True(t, os.IsNotExist(err))
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
