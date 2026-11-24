package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	episodeService "github.com/jusoaresg/gorgon/internal/episode/service"
	episodeTorrentModel "github.com/jusoaresg/gorgon/internal/episode_torrent/model"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type spyDownloader struct {
	downloadCalled        bool
	downloadTorrentCalled bool
	upgradeCalled         bool
	upgradeInfo           []episodeService.UpgradeInfo
	lastResponse          schema.SearchResponse
}

func (s *spyDownloader) DownloadEpisode(ep episodeModel.Episode, response schema.SearchResponse, score ...int) error {
	s.downloadCalled = true
	s.lastResponse = response
	return nil
}

func (s *spyDownloader) DownloadEpisodeTorrent(ep episodeModel.Episode, response schema.SearchResponse, score ...int) error {
	s.downloadTorrentCalled = true
	s.lastResponse = response
	return nil
}

func (s *spyDownloader) UpgradeEpisode(ep episodeModel.Episode, response schema.SearchResponse, newScore int, upgrade ...episodeService.UpgradeInfo) error {
	s.upgradeCalled = true
	s.lastResponse = response
	s.upgradeInfo = upgrade
	return nil
}

func setupManualDownloadHandler(t *testing.T, tracking string, existingHash string) (*Handler, *spyDownloader, int64) {
	t.Helper()
	db := testutils.GetTestDB()

	showRepo := showRepository.NewShowRepository(db)
	seasonRepo := seasonRepository.NewSeasonRepository(db)
	epRepo := episodeRepository.NewEpisodeRepository(db)
	epTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(db)

	showID, err := showRepo.Create(testutils.MakeFakeShow())
	require.NoError(t, err)

	season := testutils.MakeFakeSeason()
	season.ShowID = showID
	seasonID, err := seasonRepo.Create(season)
	require.NoError(t, err)

	ep := testutils.MakeFakeEpisode()
	ep.ShowID = showID
	ep.SeasonID = seasonID
	ep.Tracking = tracking
	epID, err := epRepo.Create(ep)
	require.NoError(t, err)

	if existingHash != "" {
		_, err := epTorrentRepo.Upsert(episodeTorrentModel.EpisodeTorrent{
			EpisodeId: epID,
			Hash:      existingHash,
			Title:     "Old.Release.720p",
			Indexer:   "OldIndexer",
			Score:     20,
		})
		require.NoError(t, err)
	}

	spy := &spyDownloader{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	searchSvc := episodeService.NewEpisodeSearchService(db, logger, nil, spy, epRepo, showRepo)

	h := &Handler{
		ShowRepo:           showRepo,
		EpisodeRepo:        epRepo,
		EpisodeTorrentRepo: epTorrentRepo,
		EpisodeSearchSvc:   searchSvc,
		DB:                 db,
	}
	return h, spy, epID
}

func TestDownloadEpisodeTorrent_DownloadedEpisodeTriggersManualUpgrade(t *testing.T) {
	h, spy, epID := setupManualDownloadHandler(t, episodeModel.TrackingDownloaded, "oldhash123")

	e := echo.New()
	body, _ := json.Marshal(map[string]string{
		"guid":     "magnet:?xt=urn:btih:newhash456",
		"infoHash": "newhash456",
		"title":    "New.Release.1080p",
		"indexer":  "NewIndexer",
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/front/episode/%d/download", epID), bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", epID))

	err := h.DownloadEpisodeTorrent(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, spy.upgradeCalled, "expected UpgradeEpisode to be called for already-downloaded episode")
	assert.False(t, spy.downloadTorrentCalled)
	require.Len(t, spy.upgradeInfo, 1)
	assert.True(t, spy.upgradeInfo[0].IsManual)
	assert.Equal(t, 20, spy.upgradeInfo[0].OldScore)
	assert.Equal(t, "Old.Release.720p", spy.upgradeInfo[0].OldReleaseName)
	assert.Contains(t, rec.Body.String(), "Upgrade started")
}

func TestDownloadEpisodeTorrent_DownloadedEpisodeRejectsSameHash(t *testing.T) {
	h, spy, epID := setupManualDownloadHandler(t, episodeModel.TrackingDownloaded, "SAMEHASH123")

	e := echo.New()
	body, _ := json.Marshal(map[string]string{
		"guid":     "magnet:?xt=urn:btih:samehash123",
		"infoHash": "samehash123",
		"title":    "Old.Release.720p",
		"indexer":  "OldIndexer",
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/front/episode/%d/download", epID), bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", epID))

	err := h.DownloadEpisodeTorrent(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, spy.upgradeCalled)
	assert.False(t, spy.downloadTorrentCalled)
	assert.Contains(t, rec.Body.String(), "already downloaded")
}

func TestDownloadEpisodeTorrent_WantedEpisodeTriggersManualDownload(t *testing.T) {
	h, spy, epID := setupManualDownloadHandler(t, episodeModel.TrackingWanted, "")

	e := echo.New()
	body, _ := json.Marshal(map[string]string{
		"guid":     "magnet:?xt=urn:btih:freshhash789",
		"infoHash": "freshhash789",
		"title":    "Fresh.Release.1080p",
		"indexer":  "TestIndexer",
	})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/front/episode/%d/download", epID), bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(fmt.Sprintf("%d", epID))

	err := h.DownloadEpisodeTorrent(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, spy.downloadTorrentCalled)
	assert.False(t, spy.upgradeCalled)
	assert.Contains(t, rec.Body.String(), "Download started")
}
