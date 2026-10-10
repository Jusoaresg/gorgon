package api

import (
	"fmt"
	"log/slog"

	tvMazeService "github.com/jusoaresg/gorgon/external/tvmaze/service"
	"github.com/jusoaresg/gorgon/internal/show/repository"
	showService "github.com/jusoaresg/gorgon/internal/show/service"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/jusoaresg/gorgon/pkg/schemas/dtos"

	"github.com/labstack/echo/v4"
)

type UpdateShowData struct {
	UpdateShow   dtos.ShowDto `json:"updatedShow"`
	ToastMessage string       `json:"toastMessage"`
}

// @BasePath /api/v1

// @Summary Update Show Info
// @Description Updates the Show info from TvMze
// @Tags Database/Show
// @Produce json
// @Param request body schemas.IdRequest true "Request Body"
// @Success 200 {object} schemas.DefaultResponse
// @Failure 400 {object} schemas.ErrorResponse
// @Failure 500 {object} schemas.ErrorResponse
// @Router /database/show/update-info [post]
func (h *Handler) UpdateShowInfo(c echo.Context) error {
	h.Logger.Info("Received request to update show info", slog.String("endpoint", "/database/show/update-info"), slog.String("method", "POST"))

	var request schemas.IdRequest
	if err := c.Bind(&request); err != nil {
		return err
	}

	showRepo := repository.NewShowRepository(h.DB)
	tvMazeSvc := tvMazeService.NewTvMazeSearchService(h.Logger)
	showManager := showService.NewShowManagerService(h.Logger, h.DB)

	updatedShow, err := UpdateSingleShowInfo(showRepo, tvMazeSvc, showManager, h.Logger, request.Id)
	if err != nil {
		h.Logger.Error("Failed to update show information", slog.Int("show_id", int(request.Id)))
		schemas.SendError(c, 500, "Failed to update show info", UpdateShowData{
			ToastMessage: "Failed to update show info",
		})
		return nil
	}

	h.Logger.Info("Show info updated successfully")
	schemas.SendSuccess(c, "Update Show Info", UpdateShowData{
		UpdateShow:   *updatedShow,
		ToastMessage: "Show info updated",
	})
	return nil
}

func UpdateSingleShowInfo(
	showRepo *repository.ShowRepository,
	tvMazeService *tvMazeService.TvMazeSearchService,
	showManager *showService.ShowManagerService,
	logger *slog.Logger,
	showId int64,
) (*dtos.ShowDto, error) {
	// Stage 1: Fetch show from local database
	show, err := showRepo.GetById(showId)
	if err != nil {
		logger.Error("Failed to fetch show from repository",
			slog.Int64("show_id", showId),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("failed to fetch show %d from database: %w", showId, err)
	}

	// Stage 2: Fetch show details from TVMaze
	showDTO, err := tvMazeService.SearchByTvMazeId(show.TvMazeID)
	if err != nil {
		logger.Error("Failed to fetch show info from TVMaze",
			slog.Int64("show_id", showId),
			slog.Int64("tvmaze_id", show.TvMazeID),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("failed to fetch show details from TVMaze for show %d (tvmaze_id %d): %w", showId, show.TvMazeID, err)
	}

	// Stage 3: Fetch episodes from TVMaze
	episodesDTO, err := showManager.GetEpisodes(show.TvMazeID)
	if err != nil {
		logger.Error("Failed to fetch episodes from TVMaze",
			slog.Int64("show_id", showId),
			slog.Int64("tvmaze_id", show.TvMazeID),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("failed to fetch episodes from TVMaze for show %d (tvmaze_id %d): %w", showId, show.TvMazeID, err)
	}

	// Stage 4: Fetch seasons from TVMaze
	seasonsDTO, err := showManager.GetSeasons(show.TvMazeID)
	if err != nil {
		logger.Error("Failed to fetch seasons from TVMaze",
			slog.Int64("show_id", showId),
			slog.Int64("tvmaze_id", show.TvMazeID),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("failed to fetch seasons from TVMaze for show %d (tvmaze_id %d): %w", showId, show.TvMazeID, err)
	}

	// Stage 5: Database transaction updating show and relations
	err = showManager.UpdateShowWithRelations(*showDTO, *seasonsDTO, *episodesDTO)
	if err != nil {
		logger.Error("Failed to update show and relations in database transaction",
			slog.Int64("show_id", showId),
			slog.Int64("tvmaze_id", show.TvMazeID),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("failed to update show and relations in database for show %d (tvmaze_id %d): %w", showId, show.TvMazeID, err)
	}

	logger.Info("Successfully updated show info and relations",
		slog.Int64("show_id", showId),
		slog.Int64("tvmaze_id", show.TvMazeID),
		slog.String("name", showDTO.Name),
	)

	return showDTO, nil
}
