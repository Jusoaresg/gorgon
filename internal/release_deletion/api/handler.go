package api

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/internal/release_deletion"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	DB     *sqlx.DB
	Logger *slog.Logger
}

func (h *Handler) service() *release_deletion.Service {
	return release_deletion.NewWithOptions(h.DB, h.Logger, release_deletion.Options{})
}
func (h *Handler) List(c echo.Context) error {
	ops, err := h.service().List()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Unable to load deletion operations"})
	}
	return c.JSON(http.StatusOK, map[string]any{"operations": ops})
}
func (h *Handler) Get(c echo.Context) error {
	op, err := h.service().Get(c.Param("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		return c.JSON(status, map[string]string{"error": "Deletion operation unavailable"})
	}
	return c.JSON(http.StatusOK, map[string]any{"operation": op})
}
func (h *Handler) Retry(c echo.Context) error {
	op, err := h.service().SubmitRetry(c.Request().Context(), c.Param("id"))
	return c.JSON(release_deletion.HTTPStatus(op, err), release_deletion.Response(op, err))
}
