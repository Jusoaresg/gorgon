package web

import (
	"errors"
	"net/http"
	"testing"

	"github.com/jusoaresg/gorgon/internal/release_deletion"
	"github.com/stretchr/testify/require"
)

func TestBlacklistDownloadDelegatesCleanupAndSearchIntent(t *testing.T) {
	coordinator := &removalCoordinator{operation: release_deletion.Operation{ID: "blacklist-op", Status: "completed", EpisodeID: 1}}
	h := &Handler{DeletionService: coordinator}
	c, rec := removeContext(" ABC ")
	require.NoError(t, h.BlacklistDownload(c))
	require.Equal(t, "abc", coordinator.request.Hash)
	require.Equal(t, "blacklist", coordinator.request.Intention)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "blacklist-op")
}
func TestBlacklistDownloadDoesNotAnnounceSearchAfterFailedCleanup(t *testing.T) {
	coordinator := &removalCoordinator{operation: release_deletion.Operation{ID: "blacklist-op", Status: "failed"}, err: errors.New("files remain")}
	h := &Handler{DeletionService: coordinator}
	c, rec := removeContext("abc")
	require.NoError(t, h.BlacklistDownload(c))
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, rec.Body.String(), "searching for another")
	require.Contains(t, rec.Body.String(), "blacklist-op")
}

func TestBlacklistDownloadPendingAndInvalidResults(t *testing.T) {
	for _, tt := range []struct {
		name, status string
		err          error
		code         int
	}{
		{"pending", "pending", nil, http.StatusAccepted},
		{"shared", "", release_deletion.ErrUnsafe, http.StatusUnprocessableEntity},
		{"conflict", "running", release_deletion.ErrConflict, http.StatusConflict},
	} {
		t.Run(tt.name, func(t *testing.T) {
			coordinator := &removalCoordinator{operation: release_deletion.Operation{ID: "op", Status: tt.status}, err: tt.err}
			h := &Handler{DeletionService: coordinator}
			c, rec := removeContext("abc")
			require.NoError(t, h.BlacklistDownload(c))
			require.Equal(t, tt.code, rec.Code)
			require.NotContains(t, rec.Body.String(), "replacement search queued")
		})
	}
	coordinator := &removalCoordinator{}
	h := &Handler{DeletionService: coordinator}
	c, rec := removeContext(" ")
	require.NoError(t, h.BlacklistDownload(c))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, release_deletion.Request{}, coordinator.request)
}
