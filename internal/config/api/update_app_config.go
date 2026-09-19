package api

import (
	"log/slog"

	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/labstack/echo/v4"
)

type UpdatedConfigResponse struct {
	NewConfig    schemas.ConfigFile `json:"newConfig"`
	ToastMessage string             `json:"toastMessage"`
}

// @BasePath /api/v1

// @Summary Update App Config
// @Description Update Gorgon Application Config
// @Tags App/Config
// @Produce json
// @Param request body schemas.ConfigFile true "Request Body"
// @Success 200 {object} schemas.DefaultResponse
// @Failure 400 {object} schemas.ErrorResponse
// @Failure 500 {object} schemas.ErrorResponse
// @Router /app/config [post]
// @Router /app/config [patch]
func (h *Handler) UpdateAppConfig(c echo.Context) error {
	h.Logger.Info("Received request to Update App Config", slog.String("endpoint", "/app/config"), slog.String("method", c.Request().Method))

	var request schemas.UpdateConfigInput

	if err := c.Bind(&request); err != nil {
		schemas.SendError(c, 400, "Failed to bind body")
		return err
	}

	updatedCfg, err := h.Service.Update(&request)
	if err != nil {
		h.Logger.Error("Failed to update app config", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Failed to save app config", UpdatedConfigResponse{
			ToastMessage: "Failed to update config",
		})
		return err
	}

	h.Logger.Info("Successfully updated app config")
	schemas.SendSuccess(c, "Update App Config", UpdatedConfigResponse{
		NewConfig:    *updatedCfg,
		ToastMessage: "Successfully updated config",
	})
	return nil
}
