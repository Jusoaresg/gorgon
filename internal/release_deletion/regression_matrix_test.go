package release_deletion

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	qs "github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	qb "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	er "github.com/jusoaresg/gorgon/internal/episode/repository"
	seasonr "github.com/jusoaresg/gorgon/internal/season/repository"
	sr "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/pkg/services"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/require"
)

type matrixFixture struct {
	service                        *Service
	db                             *sqlx.DB
	source, link, downloads, shows string
	episodeID                      int64
	mu                             sync.Mutex
	deleted                        bool
	mode                           string
	fileName                       string
	contentPath                    string
}

func newMatrixFixture(t *testing.T, mode string) *matrixFixture {
	t.Helper()
	root := t.TempDir()
	f := &matrixFixture{downloads: filepath.Join(root, "downloads"), shows: filepath.Join(root, "shows"), mode: mode, fileName: "movie.mkv"}
	require.NoError(t, os.MkdirAll(f.downloads, 0755))
	require.NoError(t, os.MkdirAll(f.shows, 0755))
	f.source = filepath.Join(f.downloads, f.fileName)
	f.link = filepath.Join(f.shows, "episode.mkv")
	require.NoError(t, os.WriteFile(f.source, []byte("video"), 0600))
	require.NoError(t, os.Link(f.source, f.link))
	f.db = testutils.GetTestDB()
	t.Cleanup(func() { f.db.Close() })
	showID, err := sr.NewShowRepository(f.db).Create(testutils.MakeFakeShow())
	require.NoError(t, err)
	season := testutils.MakeFakeSeason()
	season.ShowID = showID
	seasonID, err := seasonr.NewSeasonRepository(f.db).Create(season)
	require.NoError(t, err)
	ep := testutils.MakeFakeEpisode()
	ep.ShowID = showID
	ep.SeasonID = seasonID
	ep.Tracking = "downloaded"
	f.episodeID, err = er.NewEpisodeRepository(f.db).Create(ep)
	require.NoError(t, err)
	_, err = f.db.Exec("INSERT INTO episode_torrents (episode_id,hash) VALUES (?,'abc')", f.episodeID)
	require.NoError(t, err)
	_, err = f.db.Exec("INSERT INTO episode_contents (episode_id,name,file_path,size) VALUES (?,'movie.mkv',?,5)", f.episodeID, f.link)
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case "/api/v2/auth/login":
			if f.mode == "auth" {
				w.WriteHeader(403)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test"})
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/info":
			if f.deleted {
				fmt.Fprint(w, "[]")
			} else {
				json.NewEncoder(w).Encode([]qs.CheckTorrentResponse{{Hash: "abc", SavePath: f.downloads, ContentPath: f.contentPath}})
			}
		case "/api/v2/torrents/files":
			json.NewEncoder(w).Encode([]map[string]any{{"name": f.fileName, "size": 5}})
		case "/api/v2/torrents/delete":
			if f.mode == "non200" {
				w.WriteHeader(500)
				return
			}
			if f.mode == "still torrent" {
				fmt.Fprint(w, "Ok.")
				return
			}
			f.deleted = true
			if f.mode != "remaining file" {
				if err := os.Remove(f.source); err != nil && !os.IsNotExist(err) {
					http.Error(w, err.Error(), 500)
					return
				}
			}
			if f.mode == "timeout effect" {
				time.Sleep(35 * time.Millisecond)
			}
			fmt.Fprint(w, "Ok.")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	api := services.NewAPIService(server.URL, logger)
	if mode == "timeout effect" {
		api.Client.Timeout = 10 * time.Millisecond
	}
	client := &qb.QBittorrentService{APIService: api, Logger: logger}
	f.service = NewWithOptions(f.db, logger, Options{Client: client, DownloadRoot: f.downloads, ShowsRoot: f.shows, ConfirmationTimeout: time.Millisecond, PollInterval: time.Millisecond})
	return f
}
func (f *matrixFixture) request() Request {
	return Request{EpisodeID: f.episodeID, Intention: "missing"}
}
func (f *matrixFixture) preserved(t *testing.T) {
	t.Helper()
	var tracking string
	require.NoError(t, f.db.Get(&tracking, "SELECT tracking FROM episodes WHERE id=?", f.episodeID))
	require.Equal(t, "downloaded", tracking)
	var count int
	require.NoError(t, f.db.Get(&count, "SELECT count(*) FROM episode_contents WHERE episode_id=?", f.episodeID))
	require.Equal(t, 1, count)
	require.NoError(t, f.db.Get(&count, "SELECT count(*) FROM episode_torrents WHERE episode_id=?", f.episodeID))
	require.Equal(t, 1, count)
	_, err := os.Lstat(f.link)
	require.NoError(t, err)
}
func TestDeletionRegressionClientFailuresPreserveRecoveryContext(t *testing.T) {
	for _, mode := range []string{"auth", "non200", "remaining file", "still torrent"} {
		t.Run(mode, func(t *testing.T) {
			f := newMatrixFixture(t, mode)
			_, err := f.service.Delete(context.Background(), f.request())
			require.Error(t, err)
			f.preserved(t)
			require.FileExists(t, f.source)
		})
	}
}
func TestDeletionRegressionTimeoutAfterEffectReconciles(t *testing.T) {
	f := newMatrixFixture(t, "timeout effect")
	op, err := f.service.Delete(context.Background(), f.request())
	require.Error(t, err)
	require.NotEmpty(t, op.ID)
	f.preserved(t)
	f.mu.Lock()
	f.mode = "success"
	f.mu.Unlock()
	op, err = f.service.Resume(context.Background(), op.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", op.Status)
	require.NoFileExists(t, f.source)
	require.NoFileExists(t, f.link)
	var tracking string
	require.NoError(t, f.db.Get(&tracking, "SELECT tracking FROM episodes WHERE id=?", f.episodeID))
	require.Equal(t, "missing", tracking)
}

func TestDeletionRegressionExtraContentPreventsSuccess(t *testing.T) {
	f := newMatrixFixture(t, "success")
	dir := filepath.Join(f.downloads, "release")
	require.NoError(t, os.Mkdir(dir, 0755))
	extra := filepath.Join(dir, "unexpected.txt")
	require.NoError(t, os.WriteFile(extra, []byte("preserve"), 0600))
	f.contentPath = dir
	op, err := f.service.Delete(context.Background(), f.request())
	require.Error(t, err)
	require.NotEqual(t, "completed", op.Status)
	require.FileExists(t, extra)
	f.preserved(t)
}
func TestDeletionRegressionBrokenSymlinkCanComplete(t *testing.T) {
	f := newMatrixFixture(t, "success")
	require.NoError(t, os.Remove(f.link))
	require.NoError(t, os.Symlink(f.source, f.link))
	require.NoError(t, os.Remove(f.source))
	op, err := f.service.Delete(context.Background(), f.request())
	require.NoError(t, err)
	require.Equal(t, "completed", op.Status)
	_, err = os.Lstat(f.link)
	require.True(t, os.IsNotExist(err))
}
func TestDeletionRegressionUnsafePathsPreserveState(t *testing.T) {
	for _, mode := range []string{"traversal", "not directory", "symlink loop", "missing hash"} {
		t.Run(mode, func(t *testing.T) {
			f := newMatrixFixture(t, "success")
			switch mode {
			case "traversal":
				f.fileName = "../outside.mkv"
				require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(f.downloads), "outside.mkv"), []byte("unrelated"), 0600))
			case "not directory":
				require.NoError(t, os.WriteFile(filepath.Join(f.downloads, "parent"), []byte("file"), 0600))
				f.fileName = "parent/child.mkv"
			case "symlink loop":
				require.NoError(t, os.Symlink("loop", filepath.Join(f.downloads, "loop")))
				f.fileName = "loop/child.mkv"
			case "missing hash":
				_, err := f.db.Exec("UPDATE episode_torrents SET hash='' WHERE episode_id=?", f.episodeID)
				require.NoError(t, err)
			}
			_, err := f.service.Delete(context.Background(), f.request())
			require.Error(t, err)
			f.preserved(t)
			require.FileExists(t, f.source)
		})
	}
}
func TestDeletionRegressionDuplicateAndConflictingIntentions(t *testing.T) {
	f := newMatrixFixture(t, "remaining file")
	first, err := f.service.Delete(context.Background(), f.request())
	require.Error(t, err)
	require.NotEmpty(t, first.ID)
	duplicate, err := f.service.Delete(context.Background(), f.request())
	require.Error(t, err)
	require.Equal(t, first.ID, duplicate.ID)
	different := f.request()
	different.Intention = "wanted"
	conflict, err := f.service.Delete(context.Background(), different)
	require.ErrorIs(t, err, ErrConflict)
	require.Equal(t, first.ID, conflict.ID)
	f.preserved(t)
}
func TestDeletionRegressionReplacedLibraryFileIsPreserved(t *testing.T) {
	f := newMatrixFixture(t, "remaining file")
	op, err := f.service.Delete(context.Background(), f.request())
	require.Error(t, err)
	require.NoError(t, os.Remove(f.source))
	replacement := filepath.Join(f.shows, "replacement.mkv")
	require.NoError(t, os.WriteFile(replacement, []byte("replacement"), 0600))
	require.NoError(t, os.Rename(replacement, f.link))
	op, err = f.service.Resume(context.Background(), op.ID)
	require.ErrorIs(t, err, ErrUnsafe)
	require.NotEqual(t, "completed", op.Status)
	content, err := os.ReadFile(f.link)
	require.NoError(t, err)
	require.Equal(t, "replacement", string(content))
	f.preserved(t)
}
func TestDeletionRegressionUnrelatedDownloadRemains(t *testing.T) {
	f := newMatrixFixture(t, "success")
	unrelated := filepath.Join(f.downloads, "unrelated.mkv")
	require.NoError(t, os.WriteFile(unrelated, []byte("other episode"), 0600))
	op, err := f.service.Delete(context.Background(), f.request())
	require.NoError(t, err)
	require.Equal(t, "completed", op.Status)
	require.FileExists(t, unrelated)
}
func TestDeletionRegressionSharedSourceAcrossDifferentHashesRejected(t *testing.T) {
	f := newMatrixFixture(t, "success")
	var epID int64
	// A second installed episode uses precisely the same download entry through
	// another library hardlink, while its recorded torrent hash differs.
	ep := testutils.MakeFakeEpisode()
	var showID, seasonID int64
	require.NoError(t, f.db.Get(&showID, "SELECT show_id FROM episodes WHERE id=?", f.episodeID))
	require.NoError(t, f.db.Get(&seasonID, "SELECT season_id FROM episodes WHERE id=?", f.episodeID))
	ep.ShowID = showID
	ep.SeasonID = seasonID
	ep.Tracking = "downloaded"
	ep.Number = 99
	var err error
	epID, err = er.NewEpisodeRepository(f.db).Create(ep)
	require.NoError(t, err)
	otherLink := filepath.Join(f.shows, "other.mkv")
	require.NoError(t, os.Link(f.source, otherLink))
	_, err = f.db.Exec("INSERT INTO episode_torrents (episode_id,hash) VALUES (?,'different')", epID)
	require.NoError(t, err)
	_, err = f.db.Exec("INSERT INTO episode_contents (episode_id,name,file_path,size) VALUES (?,'movie.mkv',?,5)", epID, otherLink)
	require.NoError(t, err)
	_, err = f.service.Delete(context.Background(), f.request())
	require.ErrorIs(t, err, ErrUnsafe)
	require.FileExists(t, f.source)
	require.FileExists(t, otherLink)
	f.preserved(t)
}

func TestDeletionRegressionInaccessibleLibraryParentRetainsRecovery(t *testing.T) {
	for _, mode := range []string{"not directory", "symlink loop"} {
		t.Run(mode, func(t *testing.T) {
			f := newMatrixFixture(t, "remaining file")
			op, err := f.service.Delete(context.Background(), f.request())
			require.Error(t, err)
			require.NoError(t, os.Remove(f.source))
			saved := f.shows + "-saved"
			require.NoError(t, os.Rename(f.shows, saved))
			if mode == "not directory" {
				require.NoError(t, os.WriteFile(f.shows, []byte("blocking file"), 0600))
			} else {
				require.NoError(t, os.Symlink(f.shows, f.shows))
			}
			resumed, err := f.service.Resume(context.Background(), op.ID)
			require.Error(t, err)
			require.NotEqual(t, "completed", resumed.Status)
			require.FileExists(t, filepath.Join(saved, "episode.mkv"))
			var tracking string
			require.NoError(t, f.db.Get(&tracking, "SELECT tracking FROM episodes WHERE id=?", f.episodeID))
			require.Equal(t, "downloaded", tracking)
			require.Equal(t, op.ID, resumed.ID)
		})
	}
}
func TestDeletionRegressionConcurrentDuplicatesShareOperation(t *testing.T) {
	f := newMatrixFixture(t, "remaining file")
	type outcome struct {
		op  Operation
		err error
	}
	results := make(chan outcome, 2)
	for i := 0; i < 2; i++ {
		go func() { op, err := f.service.Delete(context.Background(), f.request()); results <- outcome{op, err} }()
	}
	a, b := <-results, <-results
	require.Error(t, a.err)
	require.Error(t, b.err)
	require.NotEmpty(t, a.op.ID)
	require.Equal(t, a.op.ID, b.op.ID)
	f.preserved(t)
}

func TestDeletionRegressionChangedFileMetadataCannotReuseCapturedIdentity(t *testing.T) {
	f := newMatrixFixture(t, "remaining file")
	op, err := f.service.Delete(context.Background(), f.request())
	require.Error(t, err)
	// Keep the same inode deliberately: a library file replaced in place after
	// preparation must not be mistaken for the captured release.
	require.NoError(t, os.WriteFile(f.link, []byte("new unrelated contents"), 0600))
	require.NoError(t, os.Remove(f.source))
	resumed, err := f.service.Resume(context.Background(), op.ID)
	require.ErrorIs(t, err, ErrUnsafe)
	require.NotEqual(t, "completed", resumed.Status)
	content, err := os.ReadFile(f.link)
	require.NoError(t, err)
	require.Equal(t, "new unrelated contents", string(content))
	f.preserved(t)
}

func TestDeletionRegressionSeparateHardlinkSourcesAreIndependent(t *testing.T) {
	f := newMatrixFixture(t, "success")
	otherSource := filepath.Join(f.downloads, "independent.mkv")
	require.NoError(t, os.Link(f.source, otherSource))
	otherLink := filepath.Join(f.shows, "independent.mkv")
	require.NoError(t, os.Link(otherSource, otherLink))
	ep := testutils.MakeFakeEpisode()
	require.NoError(t, f.db.Get(&ep.ShowID, "SELECT show_id FROM episodes WHERE id=?", f.episodeID))
	require.NoError(t, f.db.Get(&ep.SeasonID, "SELECT season_id FROM episodes WHERE id=?", f.episodeID))
	ep.Number = 99
	ep.Tracking = "downloaded"
	otherID, err := er.NewEpisodeRepository(f.db).Create(ep)
	require.NoError(t, err)
	_, err = f.db.Exec("INSERT INTO episode_torrents (episode_id,hash) VALUES (?,'independent')", otherID)
	require.NoError(t, err)
	_, err = f.db.Exec("INSERT INTO episode_contents (episode_id,name,file_path,size) VALUES (?,'independent.mkv',?,5)", otherID, otherLink)
	require.NoError(t, err)
	op, err := f.service.Delete(context.Background(), f.request())
	require.NoError(t, err)
	require.Equal(t, "completed", op.Status)
	require.FileExists(t, otherSource)
	require.FileExists(t, otherLink)
	require.NoFileExists(t, f.source)
}
