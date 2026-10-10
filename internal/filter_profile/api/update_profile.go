package api

import (
	"errors"
	"log/slog"
	"strconv"

	filterProfileModel "github.com/jusoaresg/gorgon/internal/filter_profile/model"
	filterProfileRepository "github.com/jusoaresg/gorgon/internal/filter_profile/repository"
	"github.com/jusoaresg/gorgon/internal/filter_profile/schema"
	"github.com/jusoaresg/gorgon/pkg/schemas"

	"github.com/labstack/echo/v4"
)

// @BasePath /api/v1

// @Summary Update Filter Profile
// @Description Update Filter Profile
// @Tags Database/FilterProfile
// @Accept json
// @Produce json
// @Param id path int true "Filter Profile ID"
// @Param request body schema.SaveFilterProfileRequest true "Filter profile data"
// @Success 200 {object} schemas.DefaultResponse
// @Failure 400 {object} schemas.ErrorResponse
// @Failure 404 {object} schemas.ErrorResponse
// @Failure 500 {object} schemas.ErrorResponse
// @Router /database/filter-profile/{id} [put]
func (h *Handler) UpdateFilterProfile(c echo.Context) error {
	h.Logger.Info("Received request to Update Filter Profile", slog.String("endpoint", "/database/filter-profile"), slog.String("method", "put"))

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		h.Logger.Error("Failed to convert id to int64", slog.String("error", err.Error()))
		schemas.SendError(c, 400, "Error while converting id to int64")
		return err
	}

	var request schema.SaveFilterProfileRequest
	if err := c.Bind(&request); err != nil {
		h.Logger.Error("Failed to bind request", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Error binding request")
		return err
	}

	if err := validateSaveProfileRequest(request); err != nil {
		h.Logger.Warn("Invalid filter profile request", slog.String("error", err.Error()))
		schemas.SendError(c, 400, err.Error())
		return err
	}

	err = h.FilterProfileRepo.Update(
		filterProfileModel.FilterProfile{
			ID:                id,
			Name:              request.Name,
			UpgradeUntilScore: request.UpgradeUntilScore,
		},
		schema.ToFilterPatterns(request.Patterns),
	)
	if err != nil {
		if errors.Is(err, filterProfileRepository.ErrFilterProfileNotFound) {
			schemas.SendError(c, 404, "Filter profile not found")
			return err
		}
		h.Logger.Error("Error while updating filter profile", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Error while updating filter profile")
		return err
	}

	h.Logger.Info("Successfully updated filter profile", slog.Int64("id", id))
	schemas.SendSuccess(c, "Update Filter Profile", map[string]int64{"id": id})
	return nil
}
