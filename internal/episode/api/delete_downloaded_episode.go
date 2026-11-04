package api

import (
	"net/http"
	"strconv"

	"github.com/jusoaresg/gorgon/internal/release_deletion"
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
	op, err := release_deletion.Dispatch(c.Request().Context(), svc, release_deletion.Request{EpisodeID: id, Intention: "skipped"})
	return c.JSON(release_deletion.HTTPStatus(op, err), release_deletion.Response(op, err))
}
