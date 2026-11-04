package release_deletion

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jmoiron/sqlx"
	qs "github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	qb "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	"github.com/jusoaresg/gorgon/pkg/services"
	"github.com/stretchr/testify/require"
	"io"
	"log/slog"
	_ "modernc.org/sqlite"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOwnedPathRejectsTraversalAndSymlinkParents(t *testing.T) {
	root := t.TempDir()
	_, err := ownedPath(root, "../outside")
	require.Error(t, err)
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(root, "escape")))
	_, err = ownedPath(root, "escape/movie.mkv")
	require.Error(t, err)
	p, err := ownedPath(root, "release/movie.mkv")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "release/movie.mkv"), p)
}

func TestDeletionPreservesLinksAndTrackingUntilFilesAreConfirmedAbsent(t *testing.T) {
	s, db, source, link, deleteFiles := fixture(t)
	op, err := s.Delete(context.Background(), Request{EpisodeID: 1, Intention: "skipped"})
	require.Error(t, err)
	require.Equal(t, "failed", op.Status)
	require.FileExists(t, link)
	var tracking string
	require.NoError(t, db.Get(&tracking, "SELECT tracking FROM episodes WHERE id=1"))
	require.Equal(t, "downloaded", tracking)
	*deleteFiles = true
	require.NoError(t, os.Remove(source)) // qBittorrent may finish asynchronous removal after its HTTP response.
	resumed, err := s.Resume(context.Background(), op.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", resumed.Status)
	_, err = os.Lstat(link)
	require.True(t, os.IsNotExist(err))
	require.NoError(t, db.Get(&tracking, "SELECT tracking FROM episodes WHERE id=1"))
	require.Equal(t, "skipped", tracking)
	again, err := s.Resume(context.Background(), op.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", again.Status)
}

func TestDeletionRejectsSharedTorrentBeforeEffects(t *testing.T) {
	s, db, source, link, _ := fixture(t)
	_, err := db.Exec("INSERT INTO episode_torrents(episode_id,hash) VALUES(2,'abc')")
	require.NoError(t, err)
	_, err = s.Delete(context.Background(), Request{EpisodeID: 1, Intention: "skipped"})
	require.ErrorIs(t, err, ErrUnsafe)
	require.FileExists(t, source)
	require.FileExists(t, link)
}

func fixture(t *testing.T) (*Service, *sqlx.DB, string, string, *bool) {
	t.Helper()
	root := t.TempDir()
	downloads := filepath.Join(root, "downloads")
	shows := filepath.Join(root, "shows")
	require.NoError(t, os.MkdirAll(downloads, 0755))
	require.NoError(t, os.MkdirAll(shows, 0755))
	source := filepath.Join(downloads, "movie.mkv")
	link := filepath.Join(shows, "episode.mkv")
	require.NoError(t, os.WriteFile(source, []byte("video"), 0600))
	require.NoError(t, os.Link(source, link))
	db, err := sqlx.Open("sqlite", filepath.Join(root, "db.sqlite"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{`CREATE TABLE episodes(id INTEGER PRIMARY KEY,show_id INTEGER,season_id INTEGER,name TEXT,summary TEXT,type TEXT,number INTEGER,season INTEGER,airstamp INTEGER,tracking TEXT,last_searched_at INTEGER)`, `CREATE TABLE shows(id INTEGER PRIMARY KEY,name TEXT)`, `CREATE TABLE episode_contents(id INTEGER PRIMARY KEY,episode_id INTEGER,name TEXT,file_path TEXT,size REAL)`, `CREATE TABLE episode_torrents(episode_id INTEGER,hash TEXT)`, `CREATE TABLE release_deletion_operations(id TEXT PRIMARY KEY,target TEXT,episode_id INTEGER,hash TEXT,intention TEXT,status TEXT,stage TEXT,manifest TEXT,attempts INTEGER,last_error TEXT,created_at INTEGER,updated_at INTEGER)`, `INSERT INTO episodes VALUES(1,1,1,'Episode','','',1,1,0,'downloaded',0)`, `INSERT INTO shows VALUES(1,'Show')`, `INSERT INTO episode_torrents VALUES(1,'abc')`} {
		_, err = db.Exec(q)
		require.NoError(t, err)
	}
	_, err = db.Exec("INSERT INTO episode_contents VALUES(1,1,'movie.mkv',?,5)", link)
	require.NoError(t, err)
	deleted := false
	deleteFiles := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test"})
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/info":
			if deleted {
				fmt.Fprint(w, "[]")
			} else {
				json.NewEncoder(w).Encode([]qs.CheckTorrentResponse{{Hash: "abc", SavePath: downloads}})
			}
		case "/api/v2/torrents/files":
			fmt.Fprint(w, `[{"name":"movie.mkv","size":5}]`)
		case "/api/v2/torrents/delete":
			require.NoError(t, r.ParseForm())
			require.Equal(t, "true", r.Form.Get("deleteFiles"))
			deleted = true
			if deleteFiles {
				require.NoError(t, os.Remove(source))
			}
			fmt.Fprint(w, "Ok.")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := &qb.QBittorrentService{APIService: services.NewAPIService(server.URL, logger), Logger: logger}
	return NewWithOptions(db, logger, Options{Client: client, DownloadRoot: downloads, ShowsRoot: shows, ConfirmationTimeout: time.Millisecond, PollInterval: time.Millisecond}), db, source, link, &deleteFiles
}
