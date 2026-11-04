package api

import (
	"context"
	"fmt"
	"github.com/jusoaresg/gorgon/internal/release_deletion"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	epContentRepository "github.com/jusoaresg/gorgon/internal/episode_content/repository"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/testutils"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
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

type deletionStub struct{ request release_deletion.Request }

func (d *deletionStub) Delete(_ context.Context, r release_deletion.Request) (release_deletion.Operation, error) {
	d.request = r
	return release_deletion.Operation{ID: "operation", Status: "completed"}, nil
}
func TestDeleteDownloadedEpisode_Success(t *testing.T) {
	h, epRepo, showRepo, seasonRepo := newEpisodeTestHandler()
	_, epID := createEpisodeWithDeps(t, epRepo, showRepo, seasonRepo)
	stub := &deletionStub{}
	h.DeletionService = stub
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(epID, 10))
	assert.NoError(t, h.DeleteDownloadedEpisode(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, epID, stub.request.EpisodeID)
	assert.Equal(t, "skipped", stub.request.Intention)
	assert.Contains(t, rec.Body.String(), "operation")
}
