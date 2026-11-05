package release_guard_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	ps "github.com/jusoaresg/gorgon/external/prowlarr/schema"
	qs "github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	qb "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	er "github.com/jusoaresg/gorgon/internal/episode/repository"
	es "github.com/jusoaresg/gorgon/internal/episode/service"
	"github.com/jusoaresg/gorgon/internal/release_deletion"
	"github.com/jusoaresg/gorgon/internal/scheduler/jobs"
	"github.com/jusoaresg/gorgon/pkg/services"
	"github.com/stretchr/testify/require"
)

func TestUpgradeAcquisitionPreservesOldReleaseUntilReadyAndImportsCandidate(t *testing.T) {
	f := guardSetup(t, false)
	candidate := filepath.Join(filepath.Dir(f.source), "candidate.mkv")
	require.NoError(t, os.WriteFile(candidate, []byte("better video"), 0600))
	var added, ready, deleted, snapshotBeforeAdd atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test"})
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/info":
			hash := r.URL.Query().Get("hashes")
			torrents := []qs.CheckTorrentResponse{}
			if hash == "candidate" && added.Load() && (r.URL.Query().Get("filter") != "completed" || ready.Load()) {
				progress := float32(0.5)
				if ready.Load() {
					progress = 1
				}
				torrents = append(torrents, qs.CheckTorrentResponse{Hash: "candidate", SavePath: filepath.Dir(candidate), Progress: progress})
			}
			if hash == "abc" && !deleted.Load() {
				torrents = append(torrents, qs.CheckTorrentResponse{Hash: "abc", SavePath: filepath.Dir(f.source)})
			}
			json.NewEncoder(w).Encode(torrents)
		case "/api/v2/torrents/files":
			if r.URL.Query().Get("hash") == "candidate" {
				fmt.Fprint(w, `[{"name":"candidate.mkv","size":12}]`)
			} else {
				fmt.Fprint(w, `[{"name":"release.mkv","size":5}]`)
			}
		case "/api/v2/torrents/add":
			var count int
			err := f.db.Get(&count, "SELECT count(*) FROM release_deletion_operations WHERE episode_id=? AND hash='abc' AND intention='upgrade' AND stage='deferred'", f.ep.ID)
			snapshotBeforeAdd.Store(err == nil && count == 1)
			added.Store(true)
			fmt.Fprint(w, `{"success_count":1}`)
		case "/api/v2/torrents/delete":
			r.ParseForm()
			if r.Form.Get("hashes") != "abc" {
				http.Error(w, "wrong deletion hash", 500)
				return
			}
			if err := os.Remove(f.source); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			deleted.Store(true)
			fmt.Fprint(w, "Ok.")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &qb.QBittorrentService{APIService: services.NewAPIService(server.URL, f.deletion.Logger), Logger: f.deletion.Logger}
	downloader := es.NewEpisodeDownloader(f.db)
	downloader.TorrentClient = client
	require.NoError(t, downloader.UpgradeEpisode(f.ep, ps.SearchResponse{InfoHash: "candidate", Guid: "magnet:?xt=urn:btih:candidate", Title: "Better release"}, 100))
	require.True(t, snapshotBeforeAdd.Load())
	require.FileExists(t, f.source)
	require.FileExists(t, f.link)
	ep, err := er.NewEpisodeRepository(f.db).GetByID(f.ep.ID)
	require.NoError(t, err)
	require.Equal(t, "snatched", ep.Tracking)
	require.NoError(t, jobs.ProcessSingleSnatchedDownloadWithDB(f.db, &ep, client))
	require.FileExists(t, f.source)
	require.FileExists(t, f.link)
	// A new coordinator represents restart while the candidate is downloading.
	options := f.options
	options.Client = client
	recovered := release_deletion.NewWithOptions(f.db, f.deletion.Logger, options)
	require.NoError(t, recovered.RecoverDue(context.Background(), time.Now()))
	ready.Store(true)
	require.NoError(t, jobs.ProcessSingleSnatchedDownloadWithDB(f.db, &ep, client))
	require.NoFileExists(t, f.source)
	require.NoFileExists(t, f.link)
	require.FileExists(t, candidate)
	saved, err := er.NewEpisodeRepository(f.db).GetByID(f.ep.ID)
	require.NoError(t, err)
	require.Equal(t, "downloaded", saved.Tracking)
	var hash string
	require.NoError(t, f.db.Get(&hash, "SELECT hash FROM episode_torrents WHERE episode_id=?", f.ep.ID))
	require.Equal(t, "candidate", hash)
	var link string
	require.NoError(t, f.db.Get(&link, "SELECT file_path FROM episode_contents WHERE episode_id=?", f.ep.ID))
	require.FileExists(t, link)
}
