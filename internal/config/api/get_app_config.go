package api

import (
	"log/slog"

	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/labstack/echo/v4"
)

// @BasePath /api/v1

// @Summary Get App Config
// @Description Get Gorgon Application Config
// @Tags App/Config
// @Produce json
// @Success 200 {object} schemas.DefaultResponse
// @Failure 500 {object} schemas.ErrorResponse
// @Router /app/config [get]
func (h *Handler) GetAppConfig(c echo.Context) error {
	h.Logger.Info("Received request to Get App Config", slog.String("endpoint", "/app/config"), slog.String("method", c.Request().Method))

	cfg := h.Service.Get()

	schemas.SendSuccess(c, "Get App Config", cfg)
	return nil
}
