package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jusoaresg/gorgon/internal/release_deletion"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

type removalCoordinator struct {
	request   release_deletion.Request
	operation release_deletion.Operation
	err       error
}

func (s *removalCoordinator) Delete(_ context.Context, r release_deletion.Request) (release_deletion.Operation, error) {
	s.request = r
	return s.operation, s.err
}
func removeContext(hash string) (echo.Context, *httptest.ResponseRecorder) {
	form := url.Values{"hash": {hash}}
	req := httptest.NewRequest(http.MethodPost, "/front/downloads/remove", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	return echo.New().NewContext(req, rec), rec
}
func TestRemoveDownloadDelegatesHashAndIntention(t *testing.T) {
	coordinator := &removalCoordinator{operation: release_deletion.Operation{ID: "op-1", EpisodeID: 12, Status: "completed", Stage: "completed"}}
	h := &Handler{DeletionService: coordinator}
	c, rec := removeContext("abc")
	require.NoError(t, h.RemoveDownload(c))
	require.Equal(t, release_deletion.Request{Hash: "abc", Intention: "skipped"}, coordinator.request)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "op-1")
	require.Contains(t, rec.Body.String(), "Download removed")
}

func TestRemoveDownloadReportsCoordinatorResults(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status string
		err    error
		code   int
	}{
		{"pending", "running", nil, http.StatusAccepted},
		{"unavailable client", "", errors.New("connection refused"), http.StatusInternalServerError},
		{"post-torrent failure", "failed", errors.New("files remain"), http.StatusInternalServerError},
		{"shared hash", "", release_deletion.ErrUnsafe, http.StatusUnprocessableEntity},
		{"conflicting intention", "running", release_deletion.ErrConflict, http.StatusConflict},
	} {
		t.Run(tt.name, func(t *testing.T) {
			coordinator := &removalCoordinator{operation: release_deletion.Operation{ID: "op-2", Status: tt.status, Stage: "confirming"}, err: tt.err}
			h := &Handler{DeletionService: coordinator}
			c, rec := removeContext(" ABC ")
			require.NoError(t, h.RemoveDownload(c))
			require.Equal(t, tt.code, rec.Code)
			require.Equal(t, release_deletion.Request{Hash: "abc", Intention: "skipped"}, coordinator.request)
			require.Contains(t, rec.Body.String(), "op-2")
			require.Contains(t, rec.Body.String(), "confirming")
			require.NotContains(t, rec.Body.String(), "Download removed")
			require.NotContains(t, rec.Body.String(), "toastMessage")
		})
	}
}

func TestRemoveDownloadMissingHashHasNoEffects(t *testing.T) {
	coordinator := &removalCoordinator{}
	h := &Handler{DeletionService: coordinator}
	c, rec := removeContext("  ")
	require.NoError(t, h.RemoveDownload(c))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, release_deletion.Request{}, coordinator.request)
}

func TestRemoveDownloadRepeatedRequestKeepsOperationIdentity(t *testing.T) {
	coordinator := &removalCoordinator{operation: release_deletion.Operation{ID: "same-operation", Status: "completed"}}
	h := &Handler{DeletionService: coordinator}
	for range 2 {
		c, rec := removeContext("abc")
		require.NoError(t, h.RemoveDownload(c))
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "same-operation")
	}
}
