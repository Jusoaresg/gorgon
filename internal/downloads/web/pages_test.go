package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jusoaresg/gorgon/config"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	episodeTorrentModel "github.com/jusoaresg/gorgon/internal/episode_torrent/model"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/testutils"
)

type mockEpisodeTorrentRepo struct {
	episodeTorrentRepository.EpisodeTorrentRepositoryInterface
	torrent episodeTorrentModel.EpisodeTorrent
	err     error
}

func (m *mockEpisodeTorrentRepo) GetByHash(hash string) (episodeTorrentModel.EpisodeTorrent, error) {
	if m.err != nil {
		return episodeTorrentModel.EpisodeTorrent{}, m.err
	}
	return m.torrent, nil
}

type mockEpisodeRepo struct {
	episodeRepository.EpisodeRepositoryInterface
	episode episodeModel.Episode
	err     error
}

func (m *mockEpisodeRepo) GetByID(id int64) (episodeModel.Episode, error) {
	if m.err != nil {
		return episodeModel.Episode{}, m.err
	}
	return m.episode, nil
}

func setupTestHandler(t *testing.T) (*Handler, *episodeRepository.EpisodeRepository, *episodeTorrentRepository.EpisodeTorrentRepository, *showRepository.ShowRepository, *seasonRepository.SeasonRepository) {
	t.Helper()
	db := testutils.GetTestDB()
	epRepo := episodeRepository.NewEpisodeRepository(db)
	epTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(db)
	showRepo := showRepository.NewShowRepository(db)
	seasonRepo := seasonRepository.NewSeasonRepository(db)

	h := &Handler{
		EpisodeRepo:        epRepo,
		EpisodeTorrentRepo: epTorrentRepo,
		DB:                 db,
	}
	return h, epRepo, epTorrentRepo, showRepo, seasonRepo
}

func createEpisodeWithDeps(t *testing.T, epRepo *episodeRepository.EpisodeRepository, showRepo *showRepository.ShowRepository, seasonRepo *seasonRepository.SeasonRepository) (int64, int64) {
	t.Helper()
	show := testutils.MakeFakeShow()
	showID, err := showRepo.Create(show)
	require.NoError(t, err)

	season := testutils.MakeFakeSeason()
	season.ShowID = showID
	season.Number = 1
	seasonID, err := seasonRepo.Create(season)
	require.NoError(t, err)

	episode := testutils.MakeFakeEpisode()
	episode.ShowID = showID
	episode.SeasonID = seasonID
	epID, err := epRepo.Create(episode)
	require.NoError(t, err)

	return showID, epID
}

func TestImportDownload_MissingHash(t *testing.T) {
	h, _, _, _, _ := setupTestHandler(t)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/front/downloads/import", nil)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.ImportDownload(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "Missing torrent hash")
}

func TestImportDownload_HashNotFound(t *testing.T) {
	h, _, _, _, _ := setupTestHandler(t)

	e := echo.New()
	form := url.Values{}
	form.Set("hash", "unknownhash123")
	req := httptest.NewRequest(http.MethodPost, "/front/downloads/import", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.ImportDownload(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "No tracked episode found for this download")
}

func TestImportDownload_EpisodeTorrentZeroEpisodeID(t *testing.T) {
	h := &Handler{
		EpisodeTorrentRepo: &mockEpisodeTorrentRepo{
			torrent: episodeTorrentModel.EpisodeTorrent{
				EpisodeId: 0,
				Hash:      "unlinkedhash456",
			},
		},
	}

	e := echo.New()
	form := url.Values{}
	form.Set("hash", "unlinkedhash456")
	req := httptest.NewRequest(http.MethodPost, "/front/downloads/import", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.ImportDownload(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "No tracked episode found for this download")
}

func TestImportDownload_EpisodeNotFound(t *testing.T) {
	h := &Handler{
		EpisodeTorrentRepo: &mockEpisodeTorrentRepo{
			torrent: episodeTorrentModel.EpisodeTorrent{
				EpisodeId: 12345,
				Hash:      "orphanedhash789",
			},
		},
		EpisodeRepo: &mockEpisodeRepo{
			err: errors.New("sql: no rows in result set"),
		},
	}

	e := echo.New()
	form := url.Values{}
	form.Set("hash", "orphanedhash789")
	req := httptest.NewRequest(http.MethodPost, "/front/downloads/import", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.ImportDownload(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "Episode not found")
}

func TestImportDownload_TorrentClientNotAvailable(t *testing.T) {
	config.InDocker = false
	config.SetConfigService(nil)

	h, epRepo, epTorrentRepo, showRepo, seasonRepo := setupTestHandler(t)

	_, epID := createEpisodeWithDeps(t, epRepo, showRepo, seasonRepo)

	torrent := episodeTorrentModel.EpisodeTorrent{
		EpisodeId: epID,
		Hash:      "validhash101",
		Title:     "Valid.Episode.1080p",
	}
	_, err := epTorrentRepo.Upsert(torrent)
	require.NoError(t, err)

	e := echo.New()
	form := url.Values{}
	form.Set("hash", "validhash101")
	req := httptest.NewRequest(http.MethodPost, "/front/downloads/import", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err = h.ImportDownload(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "Torrent client not available")
}
