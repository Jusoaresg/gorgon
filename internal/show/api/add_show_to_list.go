package api

import (
	"log/slog"
	"strconv"

	"github.com/jusoaresg/gorgon/external/tvmaze/service"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	filterSettingsModel "github.com/jusoaresg/gorgon/internal/filter_settings/model"
	filterSettingsRepository "github.com/jusoaresg/gorgon/internal/filter_settings/repository"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	"github.com/jusoaresg/gorgon/internal/show/model"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	showSchema "github.com/jusoaresg/gorgon/internal/show/schema"
	showManager "github.com/jusoaresg/gorgon/internal/show/service"
	showAliasRepository "github.com/jusoaresg/gorgon/internal/show_aliases/repository"
	showSettingsModel "github.com/jusoaresg/gorgon/internal/show_settings/model"
	showSettingsRepository "github.com/jusoaresg/gorgon/internal/show_settings/repository"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/jusoaresg/gorgon/pkg/schemas/dtos"
	"github.com/jusoaresg/gorgon/pkg/services"

	"github.com/labstack/echo/v4"
)

// @BasePath /api/v1

// @Summary Add Show
// @Description Add Show to List, tracking must be "all", "future", or "none"
// @Tags Database/Show
// @Accept json
// @Produce json
// @Param request body showSchema.AddShowToListRequest true "Request Body"
// @Success 200 {object} schemas.DefaultResponse
// @Failure 400 {object} schemas.ErrorResponse
// @Failure 500 {object} schemas.ErrorResponse
// @Router /database/show [post]
func (h *Handler) AddShowToList(c echo.Context) error {
	h.Logger.Info("Received request to Add Show To List", slog.String("endpoint", "/api/v1/database/show"), slog.String("method", "POST"))

	var request showSchema.AddShowToListRequest

	if err := c.Bind(&request); err != nil {
		h.Logger.Error("Failed to bind body request")
		schemas.SendError(c, 500, "Failed to bind body request")
		return err
	}

	showModel, err := h.addShowToListHandler(c, &request)
	if err != nil {
		return err
	}

	schemas.SendSuccess(c, "Add Show To List", showModel)
	return nil
}

func (h *Handler) addShowToListHandler(c echo.Context, request *showSchema.AddShowToListRequest) (*model.Show, error) {

	validTrackings := map[string]bool{
		"all":    true,
		"future": true,
		"none":   true,
	}

	if !validTrackings[request.TrackingType] {
		schemas.SendError(c, 400, "Invalid tracking type: must be 'all', 'future', or 'none'")
		return nil, echo.NewHTTPError(400, "Invalid tracking type")
	}

	tvMazeService := service.NewTvMazeSearchService(h.Logger)
	showManagerService := showManager.NewShowManagerService(h.Logger, h.DB)

	idString := strconv.Itoa(request.Id)
	id64, err := strconv.ParseInt(idString, 10, 64)
	if err != nil {
		schemas.SendError(c, 500, err.Error())
		return nil, err
	}

	showDto, err := tvMazeService.SearchByTvMazeId(id64)
	if err != nil {
		schemas.SendError(c, 500, err.Error())
		return nil, err
	}

	episodesDto, err := showManagerService.GetEpisodes(showDto.TvMazeID)
	if err != nil {
		schemas.SendError(c, 500, err.Error())
		return nil, err
	}

	services.ApplyTrackingToEpisodes(episodesDto, request.TrackingType)

	seasonsDto, err := showManagerService.GetSeasons(showDto.TvMazeID)
	if err != nil {
		schemas.SendError(c, 500, err.Error())
		return nil, err
	}

	show := showDto.ToModel()

	tx, err := h.DB.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	showRepo := showRepository.NewShowRepository(h.DB)
	showID, err := showRepo.CreateTx(tx, show)
	if err != nil {
		h.Logger.Error("Failed to add show to database", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Failed to add show to database")
		return nil, err
	}
	show.ID = showID

	filterSettingsRepo := filterSettingsRepository.NewFilterSettingsRepository(h.DB)
	globalSettings, err := filterSettingsRepo.Get()
	if err != nil {
		globalSettings = filterSettingsModel.DefaultFilterSettings()
	}

	showSettingsRepo := showSettingsRepository.NewShowSettingsRepository(h.DB)
	showSettings := showSettingsModel.ShowSettings{
		ShowID:          showID,
		FilterProfileID: globalSettings.DefaultFilterProfileID,
		ShowType:        showSettingsModel.DetectShowType(showDto.Genres, showDto.Type),
		UseAliases:      globalSettings.UseAliases,
		OnlyLatin:       globalSettings.OnlyLatin,
	}
	if err := showSettingsRepo.UpsertTx(tx, showSettings); err != nil {
		h.Logger.Error("Failed to add show settings to database", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Failed to add show settings to database")
		return nil, err
	}

	aliasRepo := showAliasRepository.NewShowAliasesRepository(h.DB)
	aliases := showDto.ToAliasModel()
	for _, alias := range aliases {
		alias.ShowID = showID
		alias.Source = "tvmaze"
		_, err := aliasRepo.CreateTx(tx, alias)
		if err != nil {
			h.Logger.Error(
				"failed to create show alias",
				slog.String("error", err.Error()),
				slog.String("alias", alias.Alias),
				slog.String("country", alias.Country),
			)
			continue
		}
	}

	seasons := dtos.SeasonDtoSliceToModel(*seasonsDto, showID)
	var episodes []episodeModel.Episode

	for _, season := range seasons {
		seasonRepo := seasonRepository.NewSeasonRepository(h.DB)
		seasonID, err := seasonRepo.CreateTx(tx, season)
		if err != nil {
			h.Logger.Error("Failed to add season to database", slog.String("error", err.Error()))
			schemas.SendError(c, 500, "Failed to add season to database")
			return nil, err
		}

		tempEpisodesDTO := *episodesDto
		for _, episode := range tempEpisodesDTO {
			if episode.Season == season.Number {
				ep := episode.ToModel(showID, seasonID)
				episodes = append(episodes, *ep)
			}
		}
	}

	for _, episode := range episodes {
		episodeRepo := episodeRepository.NewEpisodeRepository(h.DB)
		if _, err := episodeRepo.CreateTx(tx, episode); err != nil {
			h.Logger.Error("Failed to add episode to database", slog.String("error", err.Error()))
			schemas.SendError(c, 500, "Failed to add episode to database")
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		h.Logger.Error("Failed to commit transaction", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Failed to commit transaction")
		return nil, err
	}

	return &show, nil
}
