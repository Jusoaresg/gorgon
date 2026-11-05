package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jusoaresg/gorgon/internal/release_deletion"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestPersistedQueryAndManagedRetryContracts(t *testing.T) {
	db := testutils.GetTestDB()
	t.Cleanup(func() { db.Close() })
	svc := release_deletion.NewWithOptions(db, nil, release_deletion.Options{})
	op, err := svc.Submit(context.Background(), release_deletion.Request{Hash: "unimported", Intention: "skipped"})
	require.NoError(t, err)
	h := &Handler{DB: db}
	e := echo.New()
	e.GET("/operations", h.List)
	e.GET("/operations/:id", h.Get)
	e.POST("/operations/:id/retry", h.Retry)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/operations/"+op.ID, nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "manifest")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/operations", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), op.ID)
	_, err = db.Exec("UPDATE release_deletion_operations SET status='failed',last_error='safe failure',retry_count=5 WHERE id=?", op.ID)
	require.NoError(t, err)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/operations/"+op.ID+"/retry", nil))
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NotContains(t, rec.Body.String(), "Release deleted")
	var body struct {
		Operation release_deletion.Operation `json:"operation"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "pending", body.Operation.Status)
	require.Equal(t, 0, body.Operation.RetryCount)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/operations/missing", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}
