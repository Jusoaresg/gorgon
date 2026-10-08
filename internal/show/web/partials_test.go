package web

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jusoaresg/gorgon/internal/show"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	showSettingsModel "github.com/jusoaresg/gorgon/internal/show_settings/model"
	showSettingsRepository "github.com/jusoaresg/gorgon/internal/show_settings/repository"
	"github.com/jusoaresg/gorgon/testutils"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddShowHTMX_Success_StandardShow(t *testing.T) {
	e := echo.New()
	form := url.Values{}
	form.Set("id", "10")
	form.Set("monitor", "none")

	req := httptest.NewRequest(http.MethodPost, "/front/add-show", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	db := testutils.GetTestDB()
	logger := slog.Default()
	deps := show.NewDependencies(db, logger)
	h := NewHandler(deps)

	err := h.AddShowHTMX(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)
	assert.Equal(t, "/", rec.Header().Get("HX-Redirect"))

	showRepo := showRepository.NewShowRepository(db)
	showModel, err := showRepo.GetByTvMazeID(10)
	require.NoError(t, err)

	showSettingsRepo := showSettingsRepository.NewShowSettingsRepository(db)
	settings, err := showSettingsRepo.GetByShowID(showModel.ID)
	require.NoError(t, err)
	assert.Equal(t, showModel.ID, settings.ShowID)
	assert.Equal(t, showSettingsModel.ShowTypeStandard, settings.ShowType)
	assert.True(t, settings.UseAliases)
	assert.True(t, settings.OnlyLatin)
}

func TestAddShowHTMX_Success_AnimeAutoDetect(t *testing.T) {
	e := echo.New()
	form := url.Values{}
	form.Set("id", "495") // Naruto (Anime)
	form.Set("monitor", "none")

	req := httptest.NewRequest(http.MethodPost, "/front/add-show", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	db := testutils.GetTestDB()
	logger := slog.Default()
	deps := show.NewDependencies(db, logger)
	h := NewHandler(deps)

	err := h.AddShowHTMX(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)
	assert.Equal(t, "/", rec.Header().Get("HX-Redirect"))

	showRepo := showRepository.NewShowRepository(db)
	showModel, err := showRepo.GetByTvMazeID(495)
	require.NoError(t, err)

	showSettingsRepo := showSettingsRepository.NewShowSettingsRepository(db)
	settings, err := showSettingsRepo.GetByShowID(showModel.ID)
	require.NoError(t, err)
	assert.Equal(t, showModel.ID, settings.ShowID)
	assert.Equal(t, showSettingsModel.ShowTypeAnime, settings.ShowType)
	assert.True(t, settings.UseAliases)
	assert.True(t, settings.OnlyLatin)
}

func TestAddShowHTMX_InvalidTracking(t *testing.T) {
	e := echo.New()
	form := url.Values{}
	form.Set("id", "10")
	form.Set("monitor", "invalid")

	req := httptest.NewRequest(http.MethodPost, "/front/add-show", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	db := testutils.GetTestDB()
	logger := slog.Default()
	deps := show.NewDependencies(db, logger)
	h := NewHandler(deps)

	err := h.AddShowHTMX(c)
	require.Error(t, err)
	httpErr, ok := err.(*echo.HTTPError)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)
}
