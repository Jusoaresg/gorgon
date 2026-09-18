package api

import (
	"database/sql"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	showSettingsModel "github.com/jusoaresg/gorgon/internal/show_settings/model"
	"github.com/jusoaresg/gorgon/internal/show_settings/schema"
	"github.com/jusoaresg/gorgon/pkg/schemas"

	"github.com/labstack/echo/v4"
)

// @BasePath /api/v1

// @Summary Update Show Settings
// @Description Update Show Settings
// @Tags Database/ShowSettings
// @Accept json
// @Produce json
// @Param id path int true "Show ID"
// @Param request body schema.ShowSettingsDto true "Show settings data"
// @Success 200 {object} schemas.DefaultResponse
// @Failure 400 {object} schemas.ErrorResponse
// @Failure 404 {object} schemas.ErrorResponse
// @Failure 500 {object} schemas.ErrorResponse
// @Router /database/show-settings/{id} [put]
func (h *Handler) UpdateShowSettings(c echo.Context) error {
	h.Logger.Info("Received request to Update Show Settings", slog.String("endpoint", "/database/show-settings"), slog.String("method", "put"))

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.Logger.Error("Failed to convert id to int64", slog.String("error", err.Error()))
		schemas.SendError(c, 400, "Error while converting id to int64")
		return err
	}

	if _, err := h.ShowRepo.GetById(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			schemas.SendError(c, 404, "Show not found")
			return err
		}
		h.Logger.Error("Error while checking show", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Error while checking show")
		return err
	}

	var request schema.ShowSettingsDto
	if err := c.Bind(&request); err != nil {
		h.Logger.Error("Failed to bind request", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Error binding request")
		return err
	}

	showType := request.ShowType
	if showType == "" {
		showType = showSettingsModel.ShowTypeStandard
	}

	err = h.ShowSettingsRepo.Upsert(showSettingsModel.ShowSettings{
		ShowID:          id,
		FilterProfileID: request.FilterProfileID,
		ShowType:        showType,
		UseAliases:      request.UseAliases,
		OnlyLatin:       request.OnlyLatin,
	})
	if err != nil {
		h.Logger.Error("Error while updating show settings", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Error while updating show settings")
		return err
	}

	err = h.ShowSearchPatternsRepo.Replace(id, cleanSearchPatterns(request.SearchPatterns))
	if err != nil {
		h.Logger.Error("Error while updating show search patterns", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Error while updating show search patterns")
		return err
	}

	h.Logger.Info("Successfully updated show settings", slog.Int64("show_id", id))
	schemas.SendSuccess(c, "Update Show Settings", map[string]int64{"show_id": id})
	return nil
}

func cleanSearchPatterns(patterns []string) []string {
	cleaned := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		trimmed := strings.TrimSpace(pattern)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}
