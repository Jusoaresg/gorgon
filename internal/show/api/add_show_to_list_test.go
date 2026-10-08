package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	showSchema "github.com/jusoaresg/gorgon/internal/show/schema"
	showSettingsModel "github.com/jusoaresg/gorgon/internal/show_settings/model"
	showSettingsRepository "github.com/jusoaresg/gorgon/internal/show_settings/repository"
	"github.com/jusoaresg/gorgon/testutils"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestAddShowToList_Success(t *testing.T) {
	e := echo.New()
	_ = e

	request := showSchema.AddShowToListRequest{
		Id:           10,
		TrackingType: "all",
	}
	requestJSON, err := json.Marshal(request)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/database/show", bytes.NewReader(requestJSON))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)

	logger := slog.Default()
	db := testutils.GetTestDB()

	h := &Handler{
		Logger: logger,
		DB:     db,
	}

	showModel, err := h.addShowToListHandler(c, &request)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)

	// Verify show_settings is automatically created with standard show_type
	showSettingsRepo := showSettingsRepository.NewShowSettingsRepository(db)
	settings, err := showSettingsRepo.GetByShowID(showModel.ID)
	assert.NoError(t, err)
	assert.Equal(t, showModel.ID, settings.ShowID)
	assert.Equal(t, showSettingsModel.ShowTypeStandard, settings.ShowType)
	assert.True(t, settings.UseAliases)
	assert.True(t, settings.OnlyLatin)
}

func TestAddShowToList_Anime_AutoDetect(t *testing.T) {
	e := echo.New()
	_ = e

	// TVMaze ID 495 is Naruto, which has "Anime" in its genres
	request := showSchema.AddShowToListRequest{
		Id:           495,
		TrackingType: "none",
	}
	requestJSON, err := json.Marshal(request)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/database/show", bytes.NewReader(requestJSON))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)

	logger := slog.Default()
	db := testutils.GetTestDB()

	h := &Handler{
		Logger: logger,
		DB:     db,
	}

	showModel, err := h.addShowToListHandler(c, &request)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)

	// Verify show_settings is automatically created and auto-detected as anime
	showSettingsRepo := showSettingsRepository.NewShowSettingsRepository(db)
	settings, err := showSettingsRepo.GetByShowID(showModel.ID)
	assert.NoError(t, err)
	assert.Equal(t, showModel.ID, settings.ShowID)
	assert.Equal(t, showSettingsModel.ShowTypeAnime, settings.ShowType)
	assert.True(t, settings.UseAliases)
	assert.True(t, settings.OnlyLatin)
}
