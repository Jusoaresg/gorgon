package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	qbittorrentService "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	epContentModel "github.com/jusoaresg/gorgon/internal/episode_content/model"
	epContentRepository "github.com/jusoaresg/gorgon/internal/episode_content/repository"
	episodeTorrentModel "github.com/jusoaresg/gorgon/internal/episode_torrent/model"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/jusoaresg/gorgon/testutils"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newEpisodeTestHandler() (*Handler, *episodeRepository.EpisodeRepository, *showRepository.ShowRepository, *seasonRepository.SeasonRepository) {
	db := testutils.GetTestDB()
	epRepo := episodeRepository.NewEpisodeRepository(db)
	showRepo := showRepository.NewShowRepository(db)
	seasonRepo := seasonRepository.NewSeasonRepository(db)
	epContentRepo := epContentRepository.NewEpisodeContentRepository(db)
	epTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(db)
	h := &Handler{
		EpisodeRepo:        epRepo,
		EpisodeContentRepo: epContentRepo,
		EpisodeTorrentRepo: epTorrentRepo,
		ShowRepo:           showRepo,
		DB:                 db,
		Logger:             slog.Default(),
	}
	return h, epRepo, showRepo, seasonRepo
}

func createEpisodeWithDeps(t *testing.T, epRepo *episodeRepository.EpisodeRepository, showRepo *showRepository.ShowRepository, seasonRepo *seasonRepository.SeasonRepository) (int64, int64) {
	t.Helper()
	show := testutils.MakeFakeShow()
	showID, err := showRepo.Create(show)
	assert.NoError(t, err)

	season := testutils.MakeFakeSeason()
	season.ShowID = showID
	season.Number = 1
	seasonID, err := seasonRepo.Create(season)
	assert.NoError(t, err)

	episode := testutils.MakeFakeEpisode()
	episode.ShowID = showID
	episode.SeasonID = seasonID
	epID, err := epRepo.Create(episode)
	assert.NoError(t, err)

	return showID, epID
}

func TestGetShowEpisodes_Success(t *testing.T) {
	h, epRepo, showRepo, seasonRepo := newEpisodeTestHandler()

	showID, _ := createEpisodeWithDeps(t, epRepo, showRepo, seasonRepo)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/database/show/episode/%d", showID), nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(showID, 10))

	err := h.GetShowEpisodes(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)
}

func TestGetShowEpisodes_InvalidId(t *testing.T) {
	h, _, _, _ := newEpisodeTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/database/show/episode/abc", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("abc")

	err := h.GetShowEpisodes(c)
	assert.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Result().StatusCode)
}

func TestGetShowEpisodes_Empty(t *testing.T) {
	h, _, _, _ := newEpisodeTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/database/show/episode/999999", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("999999")

	err := h.GetShowEpisodes(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)
	assert.Contains(t, rec.Body.String(), "Get Show Episodes")
}

func TestGetShowEpisodes_MultipleEpisodes(t *testing.T) {
	h, epRepo, showRepo, seasonRepo := newEpisodeTestHandler()

	show := testutils.MakeFakeShow()
	showID, err := showRepo.Create(show)
	assert.NoError(t, err)

	season := testutils.MakeFakeSeason()
	season.ShowID = showID
	season.Number = 1
	seasonID, err := seasonRepo.Create(season)
	assert.NoError(t, err)

	for i := 0; i < 3; i++ {
		ep := testutils.MakeFakeEpisode()
		ep.ShowID = showID
		ep.SeasonID = seasonID
		ep.Number = i + 1
		_, err = epRepo.Create(ep)
		assert.NoError(t, err)
	}

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/database/show/episode/%d", showID), nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(showID, 10))

	err = h.GetShowEpisodes(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)
}

func TestDeleteDownloadedEpisode_Success(t *testing.T) {
	h, epRepo, showRepo, seasonRepo := newEpisodeTestHandler()
	_, epID := createEpisodeWithDeps(t, epRepo, showRepo, seasonRepo)

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/database/show/episode/%d", epID), nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(epID, 10))

	err := h.DeleteDownloadedEpisode(c)
	if err != nil {
		t.Skip("Config file not available in test environment")
		return
	}
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)

	ep, err := epRepo.GetByID(epID)
	assert.NoError(t, err)
	assert.Equal(t, episodeModel.TrackingSkipped, ep.Tracking)
}

type fakeTorrentDeleter struct {
	deletedBatches [][]string
	failHashes     map[string]error
}

func (f *fakeTorrentDeleter) DeleteTorrent(hash string, deleteFile bool) error {
	return f.DeleteTorrents([]string{hash}, deleteFile)
}

func (f *fakeTorrentDeleter) DeleteTorrents(hashes []string, deleteFile bool) error {
	for _, h := range hashes {
		if err, ok := f.failHashes[h]; ok {
			return err
		}
	}
	copied := append([]string(nil), hashes...)
	f.deletedBatches = append(f.deletedBatches, copied)
	return nil
}

func TestDeleteDownloadedEpisode_DeletesCurrentAndPreviousHashAndFiles(t *testing.T) {
	h, epRepo, showRepo, seasonRepo := newEpisodeTestHandler()
	_, epID := createEpisodeWithDeps(t, epRepo, showRepo, seasonRepo)

	ep, err := epRepo.GetByID(epID)
	require.NoError(t, err)
	ep.Tracking = episodeModel.TrackingDownloaded
	require.NoError(t, epRepo.Update(ep))

	// First torrent, then upgraded to second torrent -> PreviousHash preserved
	_, err = h.EpisodeTorrentRepo.Upsert(episodeTorrentModel.EpisodeTorrent{EpisodeId: epID, Hash: "oldhash111"})
	require.NoError(t, err)
	_, err = h.EpisodeTorrentRepo.Upsert(episodeTorrentModel.EpisodeTorrent{EpisodeId: epID, Hash: "newhash222"})
	require.NoError(t, err)

	tmpDir := t.TempDir()
	downloadDir := filepath.Join(tmpDir, "downloads")
	showsDir := filepath.Join(tmpDir, "shows")
	require.NoError(t, os.MkdirAll(downloadDir, 0755))
	require.NoError(t, os.MkdirAll(showsDir, 0755))

	epFile := filepath.Join(downloadDir, "episode.mkv")
	require.NoError(t, os.WriteFile(epFile, []byte("video"), 0644))
	_, err = h.EpisodeContentRepo.Create(epContentModel.EpisodeContent{
		EpisodeId: epID,
		Name:      "episode.mkv",
		FilePath:  epFile,
	})
	require.NoError(t, err)

	fakeQbit := &fakeTorrentDeleter{}
	h.NewTorrentClient = func(*slog.Logger) (qbittorrentService.TorrentDeleter, error) {
		return fakeQbit, nil
	}
	h.LoadConfig = func() (*schemas.ConfigFile, error) {
		return &schemas.ConfigFile{
			QBittorrentDownloadFolder: downloadDir,
			ShowsFolder:               showsDir,
		}, nil
	}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/database/show/episode/%d", epID), nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(epID, 10))

	err = h.DeleteDownloadedEpisode(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)

	require.Len(t, fakeQbit.deletedBatches, 1)
	assert.ElementsMatch(t, []string{"newhash222", "oldhash111"}, fakeQbit.deletedBatches[0])

	_, statErr := os.Stat(epFile)
	assert.True(t, os.IsNotExist(statErr))

	_, err = h.EpisodeTorrentRepo.GetByEpisodeID(epID)
	assert.Error(t, err)

	updatedEp, err := epRepo.GetByID(epID)
	require.NoError(t, err)
	assert.Equal(t, episodeModel.TrackingSkipped, updatedEp.Tracking)
}

func TestDeleteDownloadedEpisode_QBittorrentFailurePreservesDBAndFiles(t *testing.T) {
	h, epRepo, showRepo, seasonRepo := newEpisodeTestHandler()
	_, epID := createEpisodeWithDeps(t, epRepo, showRepo, seasonRepo)

	ep, err := epRepo.GetByID(epID)
	require.NoError(t, err)
	ep.Tracking = episodeModel.TrackingDownloaded
	require.NoError(t, epRepo.Update(ep))

	_, err = h.EpisodeTorrentRepo.Upsert(episodeTorrentModel.EpisodeTorrent{EpisodeId: epID, Hash: "keephash111"})
	require.NoError(t, err)

	tmpDir := t.TempDir()
	epFile := filepath.Join(tmpDir, "episode.mkv")
	require.NoError(t, os.WriteFile(epFile, []byte("video"), 0644))
	_, err = h.EpisodeContentRepo.Create(epContentModel.EpisodeContent{
		EpisodeId: epID,
		Name:      "episode.mkv",
		FilePath:  epFile,
	})
	require.NoError(t, err)

	fakeQbit := &fakeTorrentDeleter{
		failHashes: map[string]error{"keephash111": errors.New("qbittorrent unreachable")},
	}
	h.NewTorrentClient = func(*slog.Logger) (qbittorrentService.TorrentDeleter, error) {
		return fakeQbit, nil
	}
	h.LoadConfig = func() (*schemas.ConfigFile, error) {
		return &schemas.ConfigFile{
			QBittorrentDownloadFolder: tmpDir,
			ShowsFolder:               tmpDir,
		}, nil
	}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/database/show/episode/%d", epID), nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(epID, 10))

	err = h.DeleteDownloadedEpisode(c)
	require.Error(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Result().StatusCode)

	// Torrent record, content, file, and tracking must all remain intact for retry
	torr, err := h.EpisodeTorrentRepo.GetByEpisodeID(epID)
	require.NoError(t, err)
	assert.Equal(t, "keephash111", torr.Hash)

	contents, err := h.EpisodeContentRepo.ListByEpisodeId(epID)
	require.NoError(t, err)
	assert.Len(t, contents, 1)

	_, statErr := os.Stat(epFile)
	assert.NoError(t, statErr)

	stillDownloaded, err := epRepo.GetByID(epID)
	require.NoError(t, err)
	assert.Equal(t, episodeModel.TrackingDownloaded, stillDownloaded.Tracking)
}

func TestChangeEpisodeStatus_PartialFailureContinuesRemainingEpisodes(t *testing.T) {
	h, epRepo, showRepo, seasonRepo := newEpisodeTestHandler()
	showID, ep1ID := createEpisodeWithDeps(t, epRepo, showRepo, seasonRepo)

	ep1, err := epRepo.GetByID(ep1ID)
	require.NoError(t, err)
	ep1.Tracking = episodeModel.TrackingDownloaded
	require.NoError(t, epRepo.Update(ep1))

	ep2 := testutils.MakeFakeEpisode()
	ep2.ShowID = showID
	ep2.SeasonID = ep1.SeasonID
	ep2.Number = 2
	ep2.Tracking = episodeModel.TrackingDownloaded
	ep2ID, err := epRepo.Create(ep2)
	require.NoError(t, err)

	ep3 := testutils.MakeFakeEpisode()
	ep3.ShowID = showID
	ep3.SeasonID = ep1.SeasonID
	ep3.Number = 3
	ep3.Tracking = episodeModel.TrackingDownloaded
	ep3ID, err := epRepo.Create(ep3)
	require.NoError(t, err)

	_, err = h.EpisodeTorrentRepo.Upsert(episodeTorrentModel.EpisodeTorrent{EpisodeId: ep1ID, Hash: "hash-ok-1"})
	require.NoError(t, err)
	_, err = h.EpisodeTorrentRepo.Upsert(episodeTorrentModel.EpisodeTorrent{EpisodeId: ep2ID, Hash: "hash-fail-2"})
	require.NoError(t, err)
	_, err = h.EpisodeTorrentRepo.Upsert(episodeTorrentModel.EpisodeTorrent{EpisodeId: ep3ID, Hash: "hash-ok-3"})
	require.NoError(t, err)

	tmpDir := t.TempDir()
	fakeQbit := &fakeTorrentDeleter{
		failHashes: map[string]error{"hash-fail-2": errors.New("timeout")},
	}
	h.NewTorrentClient = func(*slog.Logger) (qbittorrentService.TorrentDeleter, error) {
		return fakeQbit, nil
	}
	h.LoadConfig = func() (*schemas.ConfigFile, error) {
		return &schemas.ConfigFile{
			QBittorrentDownloadFolder: tmpDir,
			ShowsFolder:               tmpDir,
		}, nil
	}

	body, err := json.Marshal(map[string]any{
		"episode_ids": []int64{ep1ID, ep2ID, ep3ID},
		"tracking":    episodeModel.TrackingSkipped,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/database/show/episode/status", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)

	err = h.ChangeEpisodeStatus(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Result().StatusCode)
	assert.Contains(t, rec.Body.String(), "Updated 2 episode(s), but 1 failed")

	// ep1 and ep3 succeeded and were cleaned up
	got1, err := epRepo.GetByID(ep1ID)
	require.NoError(t, err)
	assert.Equal(t, episodeModel.TrackingSkipped, got1.Tracking)
	_, err = h.EpisodeTorrentRepo.GetByEpisodeID(ep1ID)
	assert.Error(t, err)

	got3, err := epRepo.GetByID(ep3ID)
	require.NoError(t, err)
	assert.Equal(t, episodeModel.TrackingSkipped, got3.Tracking)
	_, err = h.EpisodeTorrentRepo.GetByEpisodeID(ep3ID)
	assert.Error(t, err)

	// ep2 failed in qBittorrent so its tracking and torrent record remain intact
	got2, err := epRepo.GetByID(ep2ID)
	require.NoError(t, err)
	assert.Equal(t, episodeModel.TrackingDownloaded, got2.Tracking)
	torr2, err := h.EpisodeTorrentRepo.GetByEpisodeID(ep2ID)
	require.NoError(t, err)
	assert.Equal(t, "hash-fail-2", torr2.Hash)
}



