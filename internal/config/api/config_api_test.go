package api_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jusoaresg/gorgon/internal/config"
	"github.com/jusoaresg/gorgon/internal/config/api"
	"github.com/jusoaresg/gorgon/internal/config/repository"
	"github.com/jusoaresg/gorgon/internal/config/service"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHandler(t *testing.T) (*api.Handler, func()) {
	db := testutils.GetTestDB()
	repo := repository.NewAppConfigRepository(db, false)
	svc, err := service.NewConfigService(repo, slog.Default())
	require.NoError(t, err)
	deps := config.NewDependencies(svc, slog.Default())
	h := api.NewHandler(deps)
	return h, func() {
		db.Close()
	}
}

func TestGetAppConfig_Success(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/app/config", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)

	err := h.GetAppConfig(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)

	var resp schemas.DefaultResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Message, "Get App Config")
}

func TestUpdateAppConfig_BindError(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/app/config", bytes.NewReader([]byte("{invalid json")))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)

	err := h.UpdateAppConfig(c)
	assert.Error(t, err)
}

func TestUpdateAppConfig_Success(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	newHost := "http://localhost:9696"
	newTime := "09:00"
	updateInput := schemas.UpdateConfigInput{
		ProwlarrHost:             &newHost,
		TelegramDailySummaryTime: &newTime,
	}
	requestJSON, err := json.Marshal(updateInput)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/app/config", bytes.NewReader(requestJSON))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)

	err = h.UpdateAppConfig(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)

	// Verify update via GET
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/app/config", nil)
	recGet := httptest.NewRecorder()
	cGet := echo.New().NewContext(reqGet, recGet)

	err = h.GetAppConfig(cGet)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, recGet.Result().StatusCode)
}
