package api

import (
	"log/slog"

	filterProfileModel "github.com/jusoaresg/gorgon/internal/filter_profile/model"
	"github.com/jusoaresg/gorgon/internal/filter_profile/schema"
	"github.com/jusoaresg/gorgon/pkg/schemas"

	"github.com/labstack/echo/v4"
)

// @BasePath /api/v1

// @Summary Create Filter Profile
// @Description Create Filter Profile
// @Tags Database/FilterProfile
// @Accept json
// @Produce json
// @Param request body schema.SaveFilterProfileRequest true "Filter profile data"
// @Success 200 {object} schemas.DefaultResponse
// @Failure 400 {object} schemas.ErrorResponse
// @Failure 500 {object} schemas.ErrorResponse
// @Router /database/filter-profile [post]
func (h *Handler) CreateFilterProfile(c echo.Context) error {
	h.Logger.Info("Received request to Create Filter Profile", slog.String("endpoint", "/database/filter-profile"), slog.String("method", "post"))

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

	id, err := h.FilterProfileRepo.Create(
		filterProfileModel.FilterProfile{
			Name:              request.Name,
			UpgradeUntilScore: request.UpgradeUntilScore,
		},
		schema.ToFilterPatterns(request.Patterns),
	)
	if err != nil {
		h.Logger.Error("Error while creating filter profile", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Error while creating filter profile")
		return err
	}

	h.Logger.Info("Successfully created filter profile", slog.Int64("id", id))
	schemas.SendSuccess(c, "Create Filter Profile", map[string]int64{"id": id})
	return nil
}
