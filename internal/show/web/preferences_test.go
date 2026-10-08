package web

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jusoaresg/gorgon/internal/show"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/jusoaresg/gorgon/views"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetShowsPreferences_NilDB(t *testing.T) {
	sort, status := GetShowsPreferences(nil)
	assert.Equal(t, DefaultShowsSort, sort)
	assert.Equal(t, DefaultShowsStatus, status)
}

func TestGetShowsPreferences_EmptyDB(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	sort, status := GetShowsPreferences(db)
	assert.Equal(t, DefaultShowsSort, sort)
	assert.Equal(t, DefaultShowsStatus, status)
}

func TestSaveAndGetShowsPreferences(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	// Initial default check
	sort, status := GetShowsPreferences(db)
	assert.Equal(t, "added", sort)
	assert.Equal(t, "", status)

	// Save valid preferences
	err := SaveShowsPreferences(db, "name", "Running")
	require.NoError(t, err)

	sort, status = GetShowsPreferences(db)
	assert.Equal(t, "name", sort)
	assert.Equal(t, "Running", status)

	// Update to next and Ended
	err = SaveShowsPreferences(db, "next", "ended")
	require.NoError(t, err)

	sort, status = GetShowsPreferences(db)
	assert.Equal(t, "next", sort)
	assert.Equal(t, "Ended", status)

	// Update back to all status
	err = SaveShowsPreferences(db, "added", "all")
	require.NoError(t, err)

	sort, status = GetShowsPreferences(db)
	assert.Equal(t, "added", sort)
	assert.Equal(t, "", status)
}

func TestSaveShowsPreferences_InvalidValues(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	err := SaveShowsPreferences(db, "name", "Running")
	require.NoError(t, err)

	// Attempt saving invalid sort and status
	err = SaveShowsPreferences(db, "unsupported_sort", "invalid_status")
	require.NoError(t, err)

	// Original values should be preserved
	sort, status := GetShowsPreferences(db)
	assert.Equal(t, "name", sort)
	assert.Equal(t, "Running", status)
}

func TestShowsRoute_PersistsAndRestoresPreferences(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	e := echo.New()
	e.Renderer = views.NewTemplate()

	logger := slog.Default()
	deps := show.NewDependencies(db, logger)
	h := NewHandler(deps)

	// Step 1: User filters by sort=next and status=Running via HTMX / query params
	req1 := httptest.NewRequest(http.MethodGet, "/shows?sort=next&status=Running", nil)
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)

	err := h.ShowsRoute(c1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec1.Code)

	// Verify saved preferences in DB
	savedSort, savedStatus := GetShowsPreferences(db)
	assert.Equal(t, "next", savedSort)
	assert.Equal(t, "Running", savedStatus)

	// Step 2: User navigates back to /shows with NO query parameters
	req2 := httptest.NewRequest(http.MethodGet, "/shows", nil)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)

	err = h.ShowsRoute(c2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec2.Code)

	// The rendered response should contain the saved preferences selected
	body := rec2.Body.String()
	assert.Contains(t, body, `<option value="next" selected>Next Ep</option>`)
	assert.Contains(t, body, `<option value="Running" selected>Running</option>`)
}
