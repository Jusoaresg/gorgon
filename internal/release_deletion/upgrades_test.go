package release_deletion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	qs "github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	qb "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	"github.com/jusoaresg/gorgon/pkg/services"
	"github.com/stretchr/testify/require"
)

func upgradeFixture(t *testing.T) (*Service, string, string, string, *bool) {
	t.Helper()
	s, _, source, link, _ := fixture(t)
	candidate := filepath.Join(filepath.Dir(source), "candidate.mkv")
	require.NoError(t, os.WriteFile(candidate, []byte("better video"), 0600))
	oldExists, removeOld := true, true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test"})
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/info":
			if r.URL.Query().Get("hashes") == "candidate" {
				json.NewEncoder(w).Encode([]qs.CheckTorrentResponse{{Hash: "candidate", SavePath: filepath.Dir(candidate), Progress: 1}})
			} else if oldExists {
				json.NewEncoder(w).Encode([]qs.CheckTorrentResponse{{Hash: "abc", SavePath: filepath.Dir(source)}})
			} else {
				fmt.Fprint(w, "[]")
			}
		case "/api/v2/torrents/files":
			if r.URL.Query().Get("hash") == "candidate" {
				fmt.Fprint(w, `[{"name":"candidate.mkv"}]`)
			} else {
				fmt.Fprint(w, `[{"name":"movie.mkv"}]`)
			}
		case "/api/v2/torrents/delete":
			require.NoError(t, r.ParseForm())
			require.Equal(t, "abc", r.Form.Get("hashes"))
			oldExists = false
			if removeOld {
				require.NoError(t, os.Remove(source))
			}
			fmt.Fprint(w, "Ok.")
		}
	}))
	t.Cleanup(server.Close)
	s.Options.Client = &qb.QBittorrentService{APIService: services.NewAPIService(server.URL, s.Logger), Logger: s.Logger}
	return s, source, link, candidate, &removeOld
}

func TestUpgradeDefersCleanupUntilCandidateReadyAndPreservesCandidate(t *testing.T) {
	s, source, link, candidate, _ := upgradeFixture(t)
	op, err := s.PrepareUpgradeSnapshotWhileGuarded(context.Background(), 1, "candidate")
	require.NoError(t, err)
	require.Equal(t, "deferred", op.Stage)
	require.FileExists(t, source)
	require.FileExists(t, link)
	_, err = s.DB.Exec("UPDATE episode_torrents SET hash='candidate';UPDATE episodes SET tracking='snatched'")
	require.NoError(t, err)
	require.NoError(t, s.RecoverDue(context.Background(), s.now()))
	require.FileExists(t, source)
	require.FileExists(t, link)
	completed, err := s.CompleteUpgradeSnapshotWhileGuarded(context.Background(), 1, "candidate")
	require.NoError(t, err)
	require.Equal(t, "completed", completed.Status)
	require.NoFileExists(t, source)
	require.NoFileExists(t, link)
	require.FileExists(t, candidate)
	var hash, tracking string
	require.NoError(t, s.DB.Get(&hash, "SELECT hash FROM episode_torrents WHERE episode_id=1"))
	require.Equal(t, "candidate", hash)
	require.NoError(t, s.DB.Get(&tracking, "SELECT tracking FROM episodes WHERE id=1"))
	require.Equal(t, "snatched", tracking)
	var count int
	require.NoError(t, s.DB.Get(&count, "SELECT count(*) FROM episode_contents WHERE episode_id=1"))
	require.Zero(t, count)
	ops, err := s.List()
	require.NoError(t, err)
	require.Empty(t, ops)
}

func TestUpgradeCannotReplaceDeferredCandidateIntention(t *testing.T) {
	s, _, _, _, _ := upgradeFixture(t)
	_, err := s.PrepareUpgradeSnapshotWhileGuarded(context.Background(), 1, "candidate")
	require.NoError(t, err)
	_, err = s.PrepareUpgradeSnapshotWhileGuarded(context.Background(), 1, "different")
	require.ErrorIs(t, err, ErrConflict)
}

func TestUpgradeRestartAbandonsSnapshotWhenSnatchNeverCommitted(t *testing.T) {
	s, source, link, _, _ := upgradeFixture(t)
	op, err := s.PrepareUpgradeSnapshotWhileGuarded(context.Background(), 1, "candidate")
	require.NoError(t, err)
	require.NoError(t, s.RecoverDue(context.Background(), s.now()))
	saved, err := s.Get(op.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", saved.Status)
	require.FileExists(t, source)
	require.FileExists(t, link)
}

func TestUpgradeFailedCleanupResumesAfterRestartWithoutDeletingCandidate(t *testing.T) {
	s, source, link, candidate, removeOld := upgradeFixture(t)
	s.Options.ConfirmationTimeout = time.Millisecond
	s.Options.PollInterval = time.Millisecond
	op, err := s.PrepareUpgradeSnapshotWhileGuarded(context.Background(), 1, "candidate")
	require.NoError(t, err)
	_, err = s.DB.Exec("UPDATE episode_torrents SET hash='candidate';UPDATE episodes SET tracking='snatched'")
	require.NoError(t, err)
	*removeOld = false
	failed, err := s.CompleteUpgradeSnapshotWhileGuarded(context.Background(), 1, "candidate")
	require.Error(t, err)
	require.Equal(t, "failed", failed.Status)
	require.FileExists(t, source)
	require.FileExists(t, link)
	require.FileExists(t, candidate)
	require.NoError(t, os.Remove(source))
	restarted := NewWithOptions(s.DB, s.Logger, s.Options)
	completed, err := restarted.Resume(context.Background(), op.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", completed.Status)
	require.NoFileExists(t, link)
	require.FileExists(t, candidate)
	var hash string
	require.NoError(t, s.DB.Get(&hash, "SELECT hash FROM episode_torrents WHERE episode_id=1"))
	require.Equal(t, "candidate", hash)
}

func TestUpgradeRefusesCleanupWhenCandidateSharesOldSource(t *testing.T) {
	s, source, link, candidate, _ := upgradeFixture(t)
	_, err := s.PrepareUpgradeSnapshotWhileGuarded(context.Background(), 1, "candidate")
	require.NoError(t, err)
	require.NoError(t, os.Remove(candidate))
	require.NoError(t, os.Link(source, candidate))
	_, err = s.DB.Exec("UPDATE episode_torrents SET hash='candidate';UPDATE episodes SET tracking='snatched'")
	require.NoError(t, err)
	_, err = s.CompleteUpgradeSnapshotWhileGuarded(context.Background(), 1, "candidate")
	require.ErrorIs(t, err, ErrUnsafe)
	require.FileExists(t, source)
	require.FileExists(t, link)
	require.FileExists(t, candidate)
}
