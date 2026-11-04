package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jusoaresg/gorgon/internal/release_deletion"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

type trackingDeletionStub struct {
	requests []release_deletion.Request
	status   string
	err      error
}

func (s *trackingDeletionStub) Delete(_ context.Context, r release_deletion.Request) (release_deletion.Operation, error) {
	s.requests = append(s.requests, r)
	return release_deletion.Operation{ID: "tracking-operation", EpisodeID: r.EpisodeID, Intention: r.Intention, Status: s.status}, s.err
}
func changeTracking(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	require.NoError(t, h.ChangeEpisodeStatus(echo.New().NewContext(req, rec)))
	return rec
}
func TestChangeTrackingRejectsInvalidRequestBeforeSideEffects(t *testing.T) {
	for _, body := range []string{`{"tracking":"invalid","episode_ids":[1]}`, `{"tracking":"wanted","episode_ids":[]}`, `{"tracking":"wanted","episode_ids":[-1]}`, `{`} {
		t.Run(body, func(t *testing.T) {
			h := &Handler{}
			require.Equal(t, http.StatusBadRequest, changeTracking(t, h, body).Code)
		})
	}
}
func TestChangeTrackingDownloadedNoOpPreservesRelease(t *testing.T) {
	h, ep, sh, se := newEpisodeTestHandler()
	_, id := createEpisodeWithDeps(t, ep, sh, se)
	_, err := h.DB.Exec("UPDATE episodes SET tracking='downloaded' WHERE id=?", id)
	require.NoError(t, err)
	_, err = h.DB.Exec("INSERT INTO episode_torrents (episode_id,hash) VALUES (?,?)", id, "keep")
	require.NoError(t, err)
	stub := &trackingDeletionStub{}
	h.DeletionService = stub
	rec := changeTracking(t, h, fmt.Sprintf(`{"tracking":"downloaded","episode_ids":[%d]}`, id))
	require.Equal(t, 200, rec.Code)
	require.Empty(t, stub.requests)
	var hash string
	require.NoError(t, h.DB.Get(&hash, "SELECT hash FROM episode_torrents WHERE episode_id=?", id))
	require.Equal(t, "keep", hash)
}
func TestChangeTrackingBatchReportsIndependentResults(t *testing.T) {
	h, ep, sh, se := newEpisodeTestHandler()
	_, id := createEpisodeWithDeps(t, ep, sh, se)
	_, other := createEpisodeWithDeps(t, ep, sh, se)
	_, err := h.DB.Exec("UPDATE episodes SET tracking='downloaded' WHERE id=?", id)
	require.NoError(t, err)
	_, err = h.DB.Exec("INSERT INTO episode_torrents (episode_id,hash) VALUES (?,?)", id, "fail")
	require.NoError(t, err)
	stub := &trackingDeletionStub{status: "failed", err: errors.New("client unavailable")}
	h.DeletionService = stub
	rec := changeTracking(t, h, fmt.Sprintf(`{"tracking":"wanted","episode_ids":[%d,%d,999999]}`, id, other))
	require.Equal(t, http.StatusMultiStatus, rec.Code)
	var response struct {
		Results []struct {
			EpisodeID int64  `json:"episode_id"`
			Status    string `json:"status"`
		}
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Len(t, response.Results, 3)
	require.Equal(t, "failed", response.Results[0].Status)
	require.Equal(t, "completed", response.Results[1].Status)
	require.Equal(t, "not_found", response.Results[2].Status)
	original, err := ep.GetByID(id)
	require.NoError(t, err)
	require.Equal(t, "downloaded", original.Tracking)
	require.Equal(t, "wanted", stub.requests[0].Intention)
}

func TestChangeTrackingPendingAndConflictingDeletionPreserveTracking(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		err          error
		expected     string
	}{{"pending", "pending", nil, "pending"}, {"conflict", "failed", release_deletion.ErrConflict, "conflict"}, {"post torrent failure", "failed", errors.New("link removal failed"), "failed"}} {
		t.Run(tc.name, func(t *testing.T) {
			h, ep, sh, se := newEpisodeTestHandler()
			_, id := createEpisodeWithDeps(t, ep, sh, se)
			_, err := h.DB.Exec("UPDATE episodes SET tracking='downloaded' WHERE id=?", id)
			require.NoError(t, err)
			_, err = h.DB.Exec("INSERT INTO episode_torrents (episode_id,hash) VALUES (?,?)", id, "release")
			require.NoError(t, err)
			stub := &trackingDeletionStub{status: tc.status, err: tc.err}
			h.DeletionService = stub
			rec := changeTracking(t, h, fmt.Sprintf(`{"tracking":"missing","episode_ids":[%d]}`, id))
			require.Contains(t, rec.Body.String(), `"status":"`+tc.expected+`"`)
			saved, err := ep.GetByID(id)
			require.NoError(t, err)
			require.Equal(t, "downloaded", saved.Tracking)
			require.Equal(t, "missing", stub.requests[0].Intention)
		})
	}
}
func TestChangeTrackingRejectsSnatchedForInstalledRelease(t *testing.T) {
	h, ep, sh, se := newEpisodeTestHandler()
	_, id := createEpisodeWithDeps(t, ep, sh, se)
	_, err := h.DB.Exec("INSERT INTO episode_torrents (episode_id,hash) VALUES (?,?)", id, "release")
	require.NoError(t, err)
	stub := &trackingDeletionStub{}
	h.DeletionService = stub
	rec := changeTracking(t, h, fmt.Sprintf(`{"tracking":"snatched","episode_ids":[%d]}`, id))
	require.Contains(t, rec.Body.String(), `"status":"conflict"`)
	require.Empty(t, stub.requests)
}

func TestChangeTrackingActiveDeletionWithoutTorrentStillDelegates(t *testing.T) {
	h, ep, sh, se := newEpisodeTestHandler()
	_, id := createEpisodeWithDeps(t, ep, sh, se)
	_, err := h.DB.Exec("UPDATE episodes SET tracking='downloaded' WHERE id=?", id)
	require.NoError(t, err)
	_, err = h.DB.Exec(`INSERT INTO release_deletion_operations (id,target,episode_id,hash,intention,status,stage,manifest,attempts,last_error,created_at,updated_at) VALUES ('pending-tracking','hash:release',?,'release','missing','failed','links','{}',1,'',1,1)`, id)
	require.NoError(t, err)
	stub := &trackingDeletionStub{status: "pending"}
	h.DeletionService = stub
	for i := 0; i < 2; i++ {
		rec := changeTracking(t, h, fmt.Sprintf(`{"tracking":"missing","episode_ids":[%d]}`, id))
		require.Equal(t, http.StatusAccepted, rec.Code)
	}
	require.Len(t, stub.requests, 2)
	require.Equal(t, "missing", stub.requests[1].Intention)
	saved, err := ep.GetByID(id)
	require.NoError(t, err)
	require.Equal(t, "downloaded", saved.Tracking)
}
