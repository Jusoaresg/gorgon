package release_guard_test

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
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	ps "github.com/jusoaresg/gorgon/external/prowlarr/schema"
	qs "github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	qb "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	cr "github.com/jusoaresg/gorgon/internal/config/repository"
	cs "github.com/jusoaresg/gorgon/internal/config/service"
	em "github.com/jusoaresg/gorgon/internal/episode/model"
	er "github.com/jusoaresg/gorgon/internal/episode/repository"
	es "github.com/jusoaresg/gorgon/internal/episode/service"
	"github.com/jusoaresg/gorgon/internal/release_deletion"
	"github.com/jusoaresg/gorgon/internal/release_guard"
	"github.com/jusoaresg/gorgon/internal/scheduler"
	"github.com/jusoaresg/gorgon/internal/scheduler/jobs"
	seasonr "github.com/jusoaresg/gorgon/internal/season/repository"
	sr "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/jusoaresg/gorgon/pkg/services"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/jusoaresg/gorgon/utils"
	"github.com/stretchr/testify/require"
)

type guardFixture struct {
	db               *sqlx.DB
	ep               em.Episode
	source, link     string
	deletion         *release_deletion.Service
	client           *qb.QBittorrentService
	options          release_deletion.Options
	entered, proceed chan struct{}
	fail             atomic.Bool
	deleted          atomic.Bool
	addStatus        atomic.Int32
	deleteRequests   atomic.Int32
}

func guardSetup(t *testing.T, blocked bool) *guardFixture {
	t.Helper()
	config.GetLogger() // Match application startup before launching workers.
	f := &guardFixture{entered: make(chan struct{}, 1), proceed: make(chan struct{})}
	if !blocked {
		close(f.proceed)
	}
	f.db = testutils.GetTestDB()
	t.Cleanup(func() { f.db.Close() })
	root := t.TempDir()
	downloads := filepath.Join(root, "downloads")
	shows := filepath.Join(root, "shows")
	require.NoError(t, os.MkdirAll(downloads, 0755))
	require.NoError(t, os.MkdirAll(shows, 0755))
	oldCfg := config.GetConfigService()
	t.Cleanup(func() { config.SetConfigService(oldCfg) })
	cfg, err := cs.NewConfigService(cr.NewAppConfigRepository(f.db, false), slog.Default())
	require.NoError(t, err)
	mode := utils.LinkModeHardlink
	_, err = cfg.Update(&schemas.UpdateConfigInput{ShowsFolder: &shows, QBittorrentDownloadFolder: &downloads, LinkMode: &mode})
	require.NoError(t, err)
	config.SetConfigService(cfg)
	showID, err := sr.NewShowRepository(f.db).Create(testutils.MakeFakeShow())
	require.NoError(t, err)
	season := testutils.MakeFakeSeason()
	season.ShowID = showID
	seasonID, err := seasonr.NewSeasonRepository(f.db).Create(season)
	require.NoError(t, err)
	f.ep = testutils.MakeFakeEpisode()
	f.ep.ShowID = showID
	f.ep.SeasonID = seasonID
	f.ep.Tracking = "downloaded"
	f.ep.ID, err = er.NewEpisodeRepository(f.db).Create(f.ep)
	require.NoError(t, err)
	f.source = filepath.Join(downloads, "release.mkv")
	f.link = filepath.Join(shows, "release.mkv")
	require.NoError(t, os.WriteFile(f.source, []byte("video"), 0600))
	require.NoError(t, os.Link(f.source, f.link))
	_, err = f.db.Exec("INSERT INTO episode_contents (episode_id,name,file_path,size) VALUES (?,'release.mkv',?,5)", f.ep.ID, f.link)
	require.NoError(t, err)
	_, err = f.db.Exec("INSERT INTO episode_torrents (episode_id,hash,title,indexer,info_url,publish_date,score,created_at) VALUES (?,'abc','','','','',0,0)", f.ep.ID)
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test"})
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/info":
			if f.deleted.Load() {
				fmt.Fprint(w, "[]")
			} else {
				json.NewEncoder(w).Encode([]qs.CheckTorrentResponse{{Hash: "abc", SavePath: downloads}})
			}
		case "/api/v2/torrents/files":
			fmt.Fprint(w, `[{"name":"release.mkv","size":5}]`)
		case "/api/v2/torrents/add":
			status := f.addStatus.Load()
			if status == 0 {
				status = 200
			}
			if status == 200 {
				f.deleted.Store(false)
			}
			w.WriteHeader(int(status))
			fmt.Fprint(w, `{"success_count":1}`)
		case "/api/v2/torrents/delete":
			f.deleteRequests.Add(1)
			select {
			case f.entered <- struct{}{}:
			default:
			}
			<-f.proceed
			if f.fail.Load() {
				http.Error(w, "failure", 500)
				return
			}
			if err := os.Remove(f.source); err != nil && !os.IsNotExist(err) {
				http.Error(w, err.Error(), 500)
				return
			}
			f.deleted.Store(true)
			fmt.Fprint(w, "Ok.")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	f.client = &qb.QBittorrentService{APIService: services.NewAPIService(server.URL, logger), Logger: logger}
	f.options = release_deletion.Options{Client: f.client, DownloadRoot: downloads, ShowsRoot: shows, ConfirmationTimeout: time.Millisecond, PollInterval: time.Millisecond}
	f.deletion = release_deletion.NewWithOptions(f.db, logger, f.options)
	return f
}
func TestPendingDeletionSerializesVerificationImportAndSnatch(t *testing.T) {
	for _, kind := range []string{"verification", "import", "snatch", "download"} {
		t.Run(kind, func(t *testing.T) {
			f := guardSetup(t, true)
			done := make(chan error, 1)
			go func() {
				_, err := f.deletion.Delete(context.Background(), release_deletion.Request{EpisodeID: f.ep.ID, Intention: "skipped"})
				done <- err
			}()
			<-f.entered
			started := make(chan struct{})
			mutated := make(chan error, 1)
			go func() {
				close(started)
				switch kind {
				case "verification":
					scheduler.VerifyEpisodeWasDeletedWithDB(f.db)
					mutated <- nil
				case "import":
					mutated <- jobs.ProcessSingleSnatchedDownloadWithDB(f.db, &f.ep, f.client)
				case "snatch":
					mutated <- es.SnatchEpisode(&f.ep, ps.SearchResponse{InfoHash: "candidate"})
				case "download":
					mutated <- es.NewEpisodeDownloader(f.db).DownloadEpisode(f.ep, ps.SearchResponse{InfoHash: "candidate"})
				}
			}()
			<-started
			close(f.proceed)
			require.NoError(t, <-done)
			err := <-mutated
			if kind != "verification" {
				require.ErrorIs(t, err, release_guard.ErrStale)
			}
			saved, err := er.NewEpisodeRepository(f.db).GetByID(f.ep.ID)
			require.NoError(t, err)
			require.Equal(t, "skipped", saved.Tracking)
			_, err = os.Lstat(f.link)
			require.True(t, os.IsNotExist(err))
			var count int
			require.NoError(t, f.db.Get(&count, "SELECT count(*) FROM episode_contents WHERE episode_id=?", f.ep.ID))
			require.Zero(t, count)
		})
	}
}
func TestFailedDeletionRemainsProtectedAcrossServiceRestart(t *testing.T) {
	f := guardSetup(t, false)
	f.fail.Store(true)
	op, err := f.deletion.Delete(context.Background(), release_deletion.Request{EpisodeID: f.ep.ID, Intention: "wanted"})
	require.Error(t, err)
	other := release_deletion.NewWithOptions(f.db, slog.Default(), f.options)
	unlock := release_guard.Lock(f.db)
	require.ErrorIs(t, release_guard.Check(f.db, f.ep.ID, ""), release_guard.ErrProtected)
	require.ErrorIs(t, release_guard.Check(f.db, 0, " ABC "), release_guard.ErrProtected)
	unlock()
	require.ErrorIs(t, jobs.ProcessSingleSnatchedDownloadWithDB(f.db, &f.ep, f.client), release_guard.ErrProtected)
	require.ErrorIs(t, es.SnatchEpisode(&f.ep, ps.SearchResponse{InfoHash: "new"}), release_guard.ErrProtected)
	scheduler.VerifyEpisodeWasDeletedWithDB(f.db)
	require.FileExists(t, f.link)
	f.fail.Store(false)
	completed, err := other.Resume(context.Background(), op.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", completed.Status)
	unlock = release_guard.Lock(f.db)
	require.NoError(t, release_guard.Check(f.db, f.ep.ID, "abc"))
	unlock()
}
func TestHashTargetProtectsCandidateAcquisition(t *testing.T) {
	f := guardSetup(t, false)
	// Model an accepted hash-target operation not associated with this episode.
	_, err := f.db.Exec(`INSERT INTO release_deletion_operations (id,target,episode_id,hash,intention,status,stage,manifest,attempts,last_error,created_at,updated_at) VALUES ('hash-only','hash:candidate',0,'candidate','skipped','failed','prepared','{}',1,'',1,1)`)
	require.NoError(t, err)
	_, err = f.db.Exec("DELETE FROM episode_contents WHERE episode_id=?", f.ep.ID)
	require.NoError(t, err)
	_, err = f.db.Exec("UPDATE episodes SET tracking='wanted' WHERE id=?", f.ep.ID)
	require.NoError(t, err)
	f.ep.Tracking = "wanted"
	require.ErrorIs(t, es.SnatchEpisode(&f.ep, ps.SearchResponse{InfoHash: " CANDIDATE "}), release_guard.ErrProtected)
	require.ErrorIs(t, es.NewEpisodeDownloader(f.db).DownloadEpisode(f.ep, ps.SearchResponse{InfoHash: "candidate"}), release_guard.ErrProtected)
}

func TestAcquisitionDoesNotDeleteTorrentThatAlreadyExistedAtAdd(t *testing.T) {
	f := guardSetup(t, false)
	f.deleted.Store(true)
	f.addStatus.Store(http.StatusConflict)
	_, err := f.db.Exec("DELETE FROM episode_contents WHERE episode_id=?", f.ep.ID)
	require.NoError(t, err)
	_, err = f.db.Exec("DELETE FROM episode_torrents WHERE episode_id=?", f.ep.ID)
	require.NoError(t, err)
	_, err = f.db.Exec("UPDATE episodes SET tracking='wanted' WHERE id=?", f.ep.ID)
	require.NoError(t, err)
	f.ep.Tracking = "wanted"
	downloader := es.NewEpisodeDownloader(f.db)
	downloader.TorrentClient = f.client
	err = downloader.DownloadEpisode(f.ep, ps.SearchResponse{InfoHash: "candidate", Guid: "magnet:?xt=urn:btih:candidate"})
	require.ErrorContains(t, err, "no cleanup ownership")
	require.Zero(t, f.deleteRequests.Load())
	require.FileExists(t, f.source)
	require.FileExists(t, f.link)
	saved, err := er.NewEpisodeRepository(f.db).GetByID(f.ep.ID)
	require.NoError(t, err)
	require.Equal(t, "wanted", saved.Tracking)
}

func TestAcquisitionRollbackUsesRecoverableCoordinatorWithoutDeadlock(t *testing.T) {
	for _, cleanupFails := range []bool{false, true} {
		t.Run(fmt.Sprint(cleanupFails), func(t *testing.T) {
			f := guardSetup(t, false)
			f.deleted.Store(true)
			f.fail.Store(cleanupFails)
			_, err := f.db.Exec("DELETE FROM episode_contents WHERE episode_id=?", f.ep.ID)
			require.NoError(t, err)
			_, err = f.db.Exec("DELETE FROM episode_torrents WHERE episode_id=?", f.ep.ID)
			require.NoError(t, err)
			_, err = f.db.Exec("UPDATE episodes SET tracking='wanted' WHERE id=?", f.ep.ID)
			require.NoError(t, err)
			f.ep.Tracking = "wanted"
			_, err = f.db.Exec(`CREATE TRIGGER reject_acquisition BEFORE INSERT ON episode_torrents BEGIN SELECT RAISE(ABORT,'snatch unavailable'); END`)
			require.NoError(t, err)
			downloader := es.NewEpisodeDownloader(f.db)
			downloader.TorrentClient = f.client
			done := make(chan error, 1)
			go func() {
				done <- downloader.DownloadEpisode(f.ep, ps.SearchResponse{InfoHash: "abc", Guid: "magnet:?xt=urn:btih:abc"})
			}()
			select {
			case err = <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("acquisition rollback deadlocked reacquiring release guard")
			}
			require.ErrorContains(t, err, "snatch unavailable")
			require.Equal(t, int32(1), f.deleteRequests.Load())
			require.FileExists(t, f.link)
			ops, listErr := f.deletion.List()
			require.NoError(t, listErr)
			require.Len(t, ops, 1)
			if cleanupFails {
				require.ErrorContains(t, err, "cleanup incomplete")
				require.NotEqual(t, "completed", ops[0].Status)
				require.FileExists(t, f.source)
			} else {
				require.Equal(t, "completed", ops[0].Status)
				require.NoFileExists(t, f.source)
			}
		})
	}
}
