package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jusoaresg/gorgon/internal/release_deletion"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/labstack/echo/v4"
)

// DeleteDownloadedEpisode delegates all destructive effects to the deletion coordinator.
func (h *Handler) DeleteDownloadedEpisode(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"error": "Invalid episode ID"})
	}
	svc := h.DeletionService
	if svc == nil {
		svc, err = release_deletion.New(h.DB, h.Logger)
		if err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]any{"error": "Torrent client is unavailable; episode was preserved"})
		}
	}
	op, err := svc.Delete(c.Request().Context(), release_deletion.Request{EpisodeID: id, Intention: "skipped"})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, release_deletion.ErrConflict) {
			status = http.StatusConflict
		}
		if errors.Is(err, release_deletion.ErrUnsafe) {
			status = http.StatusUnprocessableEntity
		}
		return c.JSON(status, map[string]any{"operation": op, "error": "Episode deletion is incomplete; original tracking and recovery context were preserved"})
	}
	schemas.SendSuccess(c, "Delete Downloaded Episode", map[string]any{"toastMessage": "Episode deleted", "operation": op})
	return nil
}
