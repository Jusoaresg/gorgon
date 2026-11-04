package api

import (
	"database/sql"
	"errors"
	"net/http"

	episodeEvents "github.com/jusoaresg/gorgon/internal/episode/events"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	"github.com/jusoaresg/gorgon/internal/episode/schema"
	"github.com/jusoaresg/gorgon/internal/release_deletion"
	"github.com/labstack/echo/v4"
)

type trackingResult struct {
	EpisodeID int64                       `json:"episode_id"`
	Status    string                      `json:"status"`
	Operation *release_deletion.Operation `json:"operation,omitempty"`
	Error     string                      `json:"error,omitempty"`
}

// ChangeEpisodeStatus preserves each installed release until the shared deletion
// coordinator completes its requested tracking transition.
func (h *Handler) ChangeEpisodeStatus(c echo.Context) error {
	var request schema.ChangeEpisodeTrackingRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}
	switch request.Tracking {
	case episodeModel.TrackingWanted, episodeModel.TrackingMissing, episodeModel.TrackingSkipped, episodeModel.TrackingDownloaded, episodeModel.TrackingSnatched:
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid tracking value"})
	}
	if len(request.EpisodeIds) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Episode IDs are required"})
	}
	for _, id := range request.EpisodeIds {
		if id <= 0 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid episode ID"})
		}
	}
	results := make([]trackingResult, 0, len(request.EpisodeIds))
	allCompleted := true
	pending := false
	for _, id := range request.EpisodeIds {
		result := h.changeTrackingTarget(c, int64(id), request.Tracking)
		results = append(results, result)
		if result.Status != "completed" {
			allCompleted = false
		}
		if result.Status == "pending" || result.Status == "running" {
			pending = true
		}
	}
	status := http.StatusOK
	message := "Tracking updated"
	if !allCompleted {
		status = http.StatusMultiStatus
		message = "Some tracking changes are incomplete"
		if len(results) == 1 && pending {
			status = http.StatusAccepted
		}
	}
	return c.JSON(status, map[string]any{"results": results, "toastMessage": message})
}

func (h *Handler) changeTrackingTarget(c echo.Context, id int64, tracking string) trackingResult {
	result := trackingResult{EpisodeID: id, Status: "failed"}
	ep, err := h.EpisodeRepo.GetByID(id)
	if err != nil {
		result.Error = "Unable to load episode"
		if errors.Is(err, sql.ErrNoRows) {
			result.Status = "not_found"
		}
		return result
	}
	// Metadata-only transitions use one transaction, retaining active operation
	// intentions even when a previous attempt has already removed the torrent.
	tx, err := h.DB.Beginx()
	if err != nil {
		result.Error = "Unable to inspect release"
		return result
	}
	defer tx.Rollback()
	var active release_deletion.Operation
	err = tx.Get(&active, "SELECT * FROM release_deletion_operations WHERE episode_id=? AND status!='completed' ORDER BY created_at DESC LIMIT 1", id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		result.Error = "Unable to inspect deletion operation"
		return result
	}
	hasActive := err == nil
	var assets int
	if err = tx.Get(&assets, "SELECT (SELECT count(*) FROM episode_torrents WHERE episode_id=?) + (SELECT count(*) FROM episode_contents WHERE episode_id=?)", id, id); err != nil {
		result.Error = "Unable to inspect release"
		return result
	}
	if hasActive || (assets > 0 && tracking != episodeModel.TrackingDownloaded) {
		_ = tx.Rollback()
		if tracking == episodeModel.TrackingDownloaded || tracking == episodeModel.TrackingSnatched {
			result.Status = "conflict"
			result.Error = "Installed releases cannot transition to this tracking value"
			return result
		}
		svc := h.DeletionService
		if svc == nil {
			svc, err = release_deletion.New(h.DB, h.Logger)
			if err != nil {
				result.Error = "Torrent client is unavailable; episode was preserved"
				return result
			}
		}
		op, deleteErr := svc.Delete(c.Request().Context(), release_deletion.Request{EpisodeID: id, Intention: tracking})
		result.Operation = &op
		result.Status = op.Status
		if deleteErr != nil {
			result.Error = "Release deletion is incomplete; tracking was preserved"
			if errors.Is(deleteErr, release_deletion.ErrConflict) {
				result.Status = "conflict"
			} else if result.Status == "" {
				result.Status = "failed"
			}
			return result
		}
		if result.Status == "completed" {
			episodeEvents.EmitEpisodeTrackingUpdatedEvent(id, tracking, "")
		}
		return result
	}
	if ep.Tracking != tracking {
		if _, err = tx.Exec("UPDATE episodes SET tracking=? WHERE id=?", tracking, id); err != nil {
			result.Error = "Unable to update tracking"
			return result
		}
	}
	if err = tx.Commit(); err != nil {
		result.Error = "Unable to update tracking"
		return result
	}
	result.Status = "completed"
	if ep.Tracking != tracking {
		episodeEvents.EmitEpisodeTrackingUpdatedEvent(id, tracking, "")
	}
	return result
}
