package web

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jusoaresg/gorgon/config"
	tvMazeSchema "github.com/jusoaresg/gorgon/external/tvmaze/schema"
	tvMazeService "github.com/jusoaresg/gorgon/external/tvmaze/service"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	filterProfileModel "github.com/jusoaresg/gorgon/internal/filter_profile/model"
	filterProfileRepository "github.com/jusoaresg/gorgon/internal/filter_profile/repository"
	filterSettingsModel "github.com/jusoaresg/gorgon/internal/filter_settings/model"
	filterSettingsRepository "github.com/jusoaresg/gorgon/internal/filter_settings/repository"
	seasonModel "github.com/jusoaresg/gorgon/internal/season/model"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	showSchema "github.com/jusoaresg/gorgon/internal/show/schema"
	showManager "github.com/jusoaresg/gorgon/internal/show/service"
	showAliasModel "github.com/jusoaresg/gorgon/internal/show_aliases/model"
	showModel "github.com/jusoaresg/gorgon/internal/show/model"
	showSearchPatternsRepository "github.com/jusoaresg/gorgon/internal/show_search_patterns/repository"
	showSettingsModel "github.com/jusoaresg/gorgon/internal/show_settings/model"
	showSettingsRepository "github.com/jusoaresg/gorgon/internal/show_settings/repository"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/jusoaresg/gorgon/pkg/schemas/dtos"
	"github.com/jusoaresg/gorgon/pkg/services"
	"github.com/labstack/echo/v4"
)

func (h *Handler) SearchShowHTMX(c echo.Context) error {
	var request schemas.NameRequest
	if err := c.Bind(&request); err != nil {
		return nil
	}

	shows := []tvMazeSchema.SearchResult{}
	query := strings.TrimSpace(request.Name)
	if query != "" {
		results, err := h.ShowManager.SearchAndEnrich(query)
		if err != nil {
			return err
		}
		if results != nil {
			shows = *results
		}
	}

	data := map[string]any{
		"Shows": shows,
		"Query": query,
	}

	return c.Render(http.StatusOK, "add-show-card", data)
}

func (h *Handler) AddShowHTMX(c echo.Context) error {
	id, err := strconv.Atoi(c.FormValue("id"))
	if err != nil {
		return err
	}

	trackingType := c.FormValue("monitor")

	request := showSchema.AddShowToListRequest{
		Id:           id,
		TrackingType: trackingType,
	}

	validTrackings := map[string]bool{
		"all":    true,
		"future": true,
		"none":   true,
	}

	if !validTrackings[request.TrackingType] {
		return echo.NewHTTPError(400, "Invalid tracking type")
	}

	logger := config.GetLogger()

	tvMazeSvc := tvMazeService.NewTvMazeSearchService(logger)
	showManagerSvc := showManager.NewShowManagerService(logger, h.DB)

	idString := strconv.Itoa(request.Id)
	id64, err := strconv.ParseInt(idString, 10, 64)
	if err != nil {
		return err
	}

	showDto, err := tvMazeSvc.SearchByTvMazeId(id64)
	if err != nil {
		return err
	}

	episodesDto, err := showManagerSvc.GetEpisodes(showDto.TvMazeID)
	if err != nil {
		return err
	}

	services.ApplyTrackingToEpisodes(episodesDto, request.TrackingType)

	seasonsDto, err := showManagerSvc.GetSeasons(showDto.TvMazeID)
	if err != nil {
		return err
	}

	show := showDto.ToModel()

	tx, err := h.DB.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	showRepo := showRepository.NewShowRepository(h.DB)
	showID, err := showRepo.CreateTx(tx, show)
	if err != nil {
		logger.Error("Failed to add show to database", slog.String("error", err.Error()))
		return err
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
		logger.Error("Failed to add show settings to database", slog.String("error", err.Error()))
		return err
	}

	aliases := showDto.ToAliasModel()
	for _, alias := range aliases {
		alias.ShowID = showID
		alias.Source = "tvmaze"
		_, err := h.ShowAliasesRepo.CreateTx(tx, alias)
		if err != nil {
			logger.Error(
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
			logger.Error("Failed to add season to database", slog.String("error", err.Error()))
			return err
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
			logger.Error("Failed to add episode to database", slog.String("error", err.Error()))
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		logger.Error("Failed to commit transaction", slog.String("error", err.Error()))
		return err
	}

	c.Response().Header().Set("HX-Redirect", "/")
	return c.NoContent(http.StatusOK)
}

type editShowModalData struct {
	Show           showModel.Show
	Profiles       []filterProfileModel.FilterProfile
	Settings       showSettingsModel.ShowSettings
	SearchPatterns []string
	Aliases        []showAliasModel.ShowAlias
}

func (h *Handler) EditShowModalHTMX(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return err
	}

	show, err := h.AggregatorService.GetShowWithRelationsById(id)
	if err != nil {
		return err
	}

	profileRepo := filterProfileRepository.NewFilterProfileRepository(h.DB)
	profiles, err := profileRepo.List()
	if err != nil {
		return err
	}

	settingsRepo := filterSettingsRepository.NewFilterSettingsRepository(h.DB)
	global, err := settingsRepo.Get()
	if err != nil {
		return err
	}

	showSettingsRepo := showSettingsRepository.NewShowSettingsRepository(h.DB)
	stored, err := showSettingsRepo.GetByShowID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			stored = showSettingsModel.ShowSettings{
				FilterProfileID: global.DefaultFilterProfileID,
				UseAliases:      global.UseAliases,
				OnlyLatin:       global.OnlyLatin,
			}
		} else {
			return err
		}
	}

	searchPatternsRepo := showSearchPatternsRepository.NewShowSearchPatternsRepository(h.DB)
	searchPatterns, err := searchPatternsRepo.GetByShowID(id)
	if err != nil {
		return err
	}

	data := editShowModalData{
		Show:           show.Show,
		Profiles:       profiles,
		Settings:       stored,
		SearchPatterns: searchPatterns,
		Aliases:        show.ShowAliases,
	}

	return c.Render(http.StatusOK, "edit-show-modal", data)
}

type seasonModal struct {
	Season     seasonModel.Season
	EpisodeIds []int
}

func (h *Handler) ChangeSeasonTrackingModal(c echo.Context) error {
	seasonIdStr := c.Param("id")
	seasonIdInt, err := strconv.Atoi(seasonIdStr)
	if err != nil {
		return err
	}

	episodes, err := h.EpisodeRepo.ListBySeasonID(seasonIdInt)
	if err != nil {
		return err
	}

	season, err := h.SeasonRepo.GetById(int64(seasonIdInt))
	if err != nil {
		return err
	}

	episodesIds := make([]int, 0, len(episodes))
	for _, e := range episodes {
		episodesIds = append(episodesIds, int(e.ID))
	}

	return c.Render(http.StatusOK, "season-tracking-modal", seasonModal{
		Season:     season,
		EpisodeIds: episodesIds,
	})
}
