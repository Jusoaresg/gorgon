package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	qbittorrentService "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	episodeTorrentModel "github.com/jusoaresg/gorgon/internal/episode_torrent/model"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	"github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/jusoaresg/gorgon/testutils"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteShow_Success(t *testing.T) {
	db := testutils.GetTestDB()
	showRepo := repository.NewShowRepository(db)

	show := testutils.MakeFakeShow()
	id, err := showRepo.Create(show)
	assert.NoError(t, err)

	var request schemas.IdRequest
	request.Id = id

	requestJSON, err := json.Marshal(request)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/database/show/%d", id), bytes.NewReader(requestJSON))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	rec := httptest.NewRecorder()

	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(id, 10))

	h := &Handler{
		ShowRepo: showRepo,
		Logger:   slog.Default(),
		DB:       db,
	}
	err = h.DeleteShow(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)
}

func TestDeleteShow_NotFound(t *testing.T) {
	db := testutils.GetTestDB()
	showRepo := repository.NewShowRepository(db)

	request := schemas.IdRequest{
		Id: 1,
	}
	requestJSON, err := json.Marshal(request)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/database/show/%d", request.Id), bytes.NewReader(requestJSON))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	rec := httptest.NewRecorder()

	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(request.Id, 10))

	h := &Handler{
		ShowRepo: showRepo,
		Logger:   slog.Default(),
		DB:       db,
	}
	err = h.DeleteShow(c)
	assert.Error(t, err)
	assert.Equal(t, 404, rec.Result().StatusCode)
}

func TestDeleteShow_IdGreaterThanZero(t *testing.T) {
	request := schemas.IdRequest{
		Id: -100,
	}

	requestJSON, err := json.Marshal(request)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/database/show/-100", bytes.NewReader(requestJSON))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	rec := httptest.NewRecorder()

	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("-100")

	db := testutils.GetTestDB()
	h := &Handler{
		ShowRepo: repository.NewShowRepository(db),
		Logger:   slog.Default(),
		DB:       db,
	}
	err = h.DeleteShow(c)
	assert.Error(t, err)
	assert.Equal(t, 400, rec.Result().StatusCode)
}

type fakeShowTorrentDeleter struct {
	deletedHashes []string
	err           error
}

func (f *fakeShowTorrentDeleter) DeleteTorrent(hash string, deleteFile bool) error {
	return f.DeleteTorrents([]string{hash}, deleteFile)
}

func (f *fakeShowTorrentDeleter) DeleteTorrents(hashes []string, deleteFile bool) error {
	if f.err != nil {
		return f.err
	}
	f.deletedHashes = append(f.deletedHashes, hashes...)
	return nil
}

func TestDeleteShow_DeletesAssociatedTorrents(t *testing.T) {
	db := testutils.GetTestDB()
	showRepo := repository.NewShowRepository(db)
	seasonRepo := seasonRepository.NewSeasonRepository(db)
	epRepo := episodeRepository.NewEpisodeRepository(db)
	epTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(db)

	showID, err := showRepo.Create(testutils.MakeFakeShow())
	require.NoError(t, err)

	season := testutils.MakeFakeSeason()
	season.ShowID = showID
	season.Number = 1
	seasonID, err := seasonRepo.Create(season)
	require.NoError(t, err)

	ep := testutils.MakeFakeEpisode()
	ep.ShowID = showID
	ep.SeasonID = seasonID
	epID, err := epRepo.Create(ep)
	require.NoError(t, err)

	_, err = epTorrentRepo.Upsert(episodeTorrentModel.EpisodeTorrent{EpisodeId: epID, Hash: "oldshowhash"})
	require.NoError(t, err)
	_, err = epTorrentRepo.Upsert(episodeTorrentModel.EpisodeTorrent{EpisodeId: epID, Hash: "newshowhash"})
	require.NoError(t, err)

	fakeQbit := &fakeShowTorrentDeleter{}
	h := &Handler{
		ShowRepo: showRepo,
		Logger:   slog.Default(),
		DB:       db,
		NewTorrentClient: func(*slog.Logger) (qbittorrentService.TorrentDeleter, error) {
			return fakeQbit, nil
		},
		LoadConfig: func() (*schemas.ConfigFile, error) {
			return &schemas.ConfigFile{}, nil
		},
	}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/database/show/%d", showID), nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(showID, 10))

	err = h.DeleteShow(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)
	assert.ElementsMatch(t, []string{"newshowhash", "oldshowhash"}, fakeQbit.deletedHashes)

	_, err = showRepo.GetById(showID)
	assert.Error(t, err)
}

func TestDeleteShow_QBittorrentFailurePreservesShow(t *testing.T) {
	db := testutils.GetTestDB()
	showRepo := repository.NewShowRepository(db)
	seasonRepo := seasonRepository.NewSeasonRepository(db)
	epRepo := episodeRepository.NewEpisodeRepository(db)
	epTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(db)

	showID, err := showRepo.Create(testutils.MakeFakeShow())
	require.NoError(t, err)

	season := testutils.MakeFakeSeason()
	season.ShowID = showID
	season.Number = 1
	seasonID, err := seasonRepo.Create(season)
	require.NoError(t, err)

	ep := testutils.MakeFakeEpisode()
	ep.ShowID = showID
	ep.SeasonID = seasonID
	epID, err := epRepo.Create(ep)
	require.NoError(t, err)

	_, err = epTorrentRepo.Upsert(episodeTorrentModel.EpisodeTorrent{EpisodeId: epID, Hash: "keephash"})
	require.NoError(t, err)

	fakeQbit := &fakeShowTorrentDeleter{err: errors.New("qbit down")}
	h := &Handler{
		ShowRepo: showRepo,
		Logger:   slog.Default(),
		DB:       db,
		NewTorrentClient: func(*slog.Logger) (qbittorrentService.TorrentDeleter, error) {
			return fakeQbit, nil
		},
	}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/database/show/%d", showID), nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(showID, 10))

	err = h.DeleteShow(c)
	require.Error(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Result().StatusCode)

	// Show and torrent association must still exist
	_, err = showRepo.GetById(showID)
	assert.NoError(t, err)
	torr, err := epTorrentRepo.GetByEpisodeID(epID)
	require.NoError(t, err)
	assert.Equal(t, "keephash", torr.Hash)
}
