package service

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jusoaresg/gorgon/internal/show/model"
	"github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShowManager_SearchAndEnrich(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	showRepo := repository.NewShowRepository(db)
	_, err := showRepo.Create(model.Show{
		TvMazeID: 169,
		Name:     "Breaking Bad",
	})
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{"score": 9.5, "show": {"id": 169, "name": "Breaking Bad"}},
			{"score": 7.0, "show": {"id": 999, "name": "Better Call Saul"}}
		]`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tvMaze := NewTvMazeSearchServiceWithURL(server.URL, logger)

	manager := NewShowManager(*tvMaze, *showRepo, logger)

	results, err := manager.SearchAndEnrich("Breaking")
	require.NoError(t, err)
	require.NotNil(t, results)
	require.Len(t, *results, 2)

	assert.Equal(t, int64(169), (*results)[0].Show.TvMazeID)
	assert.True(t, (*results)[0].IsAdded, "show 169 exists in DB and should have IsAdded = true")

	assert.Equal(t, int64(999), (*results)[1].Show.TvMazeID)
	assert.False(t, (*results)[1].IsAdded, "show 999 does not exist in DB and should have IsAdded = false")
}

func TestShowManager_SearchAndEnrich_Error(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	showRepo := repository.NewShowRepository(db)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tvMaze := NewTvMazeSearchServiceWithURL(server.URL, logger)

	manager := NewShowManager(*tvMaze, *showRepo, logger)

	results, err := manager.SearchAndEnrich("Breaking")
	assert.Error(t, err)
	assert.Nil(t, results)
}
