package service

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTvMazeSearchService_SearchByName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/search/shows", r.URL.Path)
		assert.Equal(t, "Breaking Bad", r.URL.Query().Get("q"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{"score": 9.5, "show": {"id": 169, "name": "Breaking Bad", "type": "Scripted"}}
		]`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewTvMazeSearchServiceWithURL(server.URL, logger)

	results, err := svc.SearchByName("Breaking Bad")
	require.NoError(t, err)
	require.NotNil(t, results)
	require.Len(t, *results, 1)
	assert.Equal(t, int64(169), (*results)[0].Show.TvMazeID)
	assert.Equal(t, "Breaking Bad", (*results)[0].Show.Name)
}

func TestTvMazeSearchService_SearchByName_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewTvMazeSearchServiceWithURL(server.URL, logger)

	results, err := svc.SearchByName("Any Show")
	assert.Error(t, err)
	assert.Nil(t, results)
}

func TestTvMazeSearchService_SearchByTvMazeId(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/shows/169", r.URL.Path)
		assert.Equal(t, "akas", r.URL.Query().Get("embed"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": 169,
			"name": "Breaking Bad",
			"type": "Scripted",
			"language": "English",
			"status": "Ended",
			"updated": 1650000000,
			"_embedded": {
				"akas": [{"name": "Alternate Title", "country": {"code": "us"}}]
			}
		}`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewTvMazeSearchServiceWithURL(server.URL, logger)

	show, err := svc.SearchByTvMazeId(169)
	require.NoError(t, err)
	require.NotNil(t, show)
	assert.Equal(t, int64(169), show.TvMazeID)
	assert.Equal(t, "Breaking Bad", show.Name)
	assert.Len(t, show.Embedded.Akas, 1)
	assert.Equal(t, "Alternate Title", show.Embedded.Akas[0].Name)
}

func TestTvMazeSearchService_SearchByTheTvDbId(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/lookup/shows", r.URL.Path)
		assert.Equal(t, "81189", r.URL.Query().Get("thetvdb"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": 169, "name": "Breaking Bad"}`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewTvMazeSearchServiceWithURL(server.URL, logger)

	show, err := svc.SearchByTheTvDbId("81189")
	require.NoError(t, err)
	require.NotNil(t, show)
	assert.Equal(t, int64(169), show.TvMazeID)
	assert.Equal(t, "Breaking Bad", show.Name)
}

func TestTvMazeSearchService_SearchByImdb(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/lookup/shows", r.URL.Path)
		assert.Equal(t, "tt0903747", r.URL.Query().Get("imdb"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": 169, "name": "Breaking Bad"}`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewTvMazeSearchServiceWithURL(server.URL, logger)

	show, err := svc.SearchByImdb("tt0903747")
	require.NoError(t, err)
	require.NotNil(t, show)
	assert.Equal(t, int64(169), show.TvMazeID)
	assert.Equal(t, "Breaking Bad", show.Name)
}

func TestTvMazeSearchService_SearchEpisodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/shows/169/episodes", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{"id": 1, "name": "Pilot", "season": 1, "number": 1, "airstamp": "2008-01-20T00:00:00+00:00"},
			{"id": 2, "name": "Cat's in the Bag...", "season": 1, "number": 2, "airstamp": "2008-01-27T00:00:00+00:00"}
		]`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewTvMazeSearchServiceWithURL(server.URL, logger)

	episodes, err := svc.SearchEpisodes(169)
	require.NoError(t, err)
	require.NotNil(t, episodes)
	require.Len(t, *episodes, 2)
	assert.Equal(t, "Pilot", (*episodes)[0].Name)
	assert.Equal(t, 1, (*episodes)[0].Number)
}

func TestTvMazeSearchService_SearchSeasons(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/shows/169/seasons", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{"id": 10, "number": 1},
			{"id": 11, "number": 2}
		]`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewTvMazeSearchServiceWithURL(server.URL, logger)

	seasons, err := svc.SearchSeasons(169)
	require.NoError(t, err)
	require.NotNil(t, seasons)
	require.Len(t, *seasons, 2)
	assert.Equal(t, 1, (*seasons)[0].Number)
	assert.Equal(t, 2, (*seasons)[1].Number)
}

func TestNewTvMazeSearchService_DefaultHTTPS(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewTvMazeSearchService(logger)
	assert.Equal(t, "https://api.tvmaze.com", svc.Url)
	assert.Equal(t, "https://api.tvmaze.com", svc.APIService.Url)
}
