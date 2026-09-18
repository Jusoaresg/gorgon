package service

import (
	"fmt"
	"log/slog"

	"github.com/jusoaresg/gorgon/external/tvmaze/service"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	seasonModel "github.com/jusoaresg/gorgon/internal/season/model"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	showAliasModel "github.com/jusoaresg/gorgon/internal/show_aliases/model"
	showAliasRepository "github.com/jusoaresg/gorgon/internal/show_aliases/repository"
	showSettingsRepository "github.com/jusoaresg/gorgon/internal/show_settings/repository"
	"github.com/jusoaresg/gorgon/pkg/schemas/dtos"
	"github.com/jusoaresg/gorgon/utils"

	"github.com/jmoiron/sqlx"
)

type ShowManagerService struct {
	ShowAggregator ShowAggregatorService
	ShowRepo       showRepository.ShowRepositoryInterface
	ShowAliasRepo  showAliasRepository.ShowAliasesRepositoryInterface
	SeasonRepo     seasonRepository.SeasonRepositoryInterface
	EpisodeRepo    episodeRepository.EpisodeRepositoryInterface
	tvMaze         *service.TvMazeSearchService
	DB             *sqlx.DB
	logger         *slog.Logger
}

func NewShowManagerService(logger *slog.Logger, db *sqlx.DB) *ShowManagerService {
	showRepo := showRepository.NewShowRepository(db)
	showAliasRepo := showAliasRepository.NewShowAliasesRepository(db)
	seasonRepo := seasonRepository.NewSeasonRepository(db)
	episodeRepo := episodeRepository.NewEpisodeRepository(db)
	settingsRepo := showSettingsRepository.NewShowSettingsRepository(db)

	return &ShowManagerService{
		ShowAggregator: *NewShowAggregatorService(
			showRepo,
			&showAliasRepo,
			episodeRepo,
			episodeTorrentRepository.NewEpisodeTorrentRepository(db),
			seasonRepo,
			&settingsRepo,
		),
		ShowRepo:      showRepo,
		ShowAliasRepo: &showAliasRepo,
		SeasonRepo:    seasonRepo,
		EpisodeRepo:   episodeRepo,
		tvMaze:        service.NewTvMazeSearchService(logger),
		logger:        logger.WithGroup("showManager"),
		DB:            db,
	}
}

// TODO: Move this two Get to another file
func (sm *ShowManagerService) GetEpisodes(tvMazeId int64) (*[]dtos.EpisodeDto, error) {
	episodesDto, err := sm.tvMaze.SearchEpisodes(tvMazeId)
	if err != nil {
		return nil, err
	}

	return episodesDto, nil
}

func (sm *ShowManagerService) GetSeasons(tvMazeId int64) (*[]dtos.SeasonDto, error) {
	seasonsDto, err := sm.tvMaze.SearchSeasons(tvMazeId)
	if err != nil {
		return nil, err
	}
	return seasonsDto, nil
}

// NOTE: Everything here will need a refactor later, there're lot of problems
// Probably will be problems with the seasons updating.
func (sm *ShowManagerService) UpdateShowWithRelations(
	showDTO dtos.ShowDto,
	seasonsDTO []dtos.SeasonDto,
	episodes []dtos.EpisodeDto,
) (err error) {
	aggregatedShow, err := sm.ShowAggregator.GetShowWithRelationsByTvMazeId(showDTO.TvMazeID)
	if err != nil {
		sm.logger.Error(
			"error while getting aggregated show from db to update show",
			slog.Int64("tv_maze_id", showDTO.TvMazeID),
			slog.String("show_name", showDTO.Name),
			slog.String("error", err.Error()),
		)
		return err
	}

	showAliasesModel := aggregatedShow.ShowAliases
	episodesModel := aggregatedShow.Episodes
	seasonsModel := aggregatedShow.Seasons

	tx, err := sm.DB.Beginx()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	showModel := showDTO.ToModel()
	showModel.ID = aggregatedShow.Show.ID

	if err = sm.ShowRepo.UpdateTxByTvMazeID(tx, showModel); err != nil {
		return err
	}

	aliasesMap := make(map[string]*showAliasModel.ShowAlias)
	for i := range showAliasesModel {
		a := &showAliasesModel[i]
		aliasesMap[a.Alias] = a
	}

	showAliasDto := showDTO.ToAliasModel()
	for _, alias := range showAliasDto {
		if existing, ok := aliasesMap[alias.Alias]; ok {
			sm.logger.Info("updating show alias", slog.String("alias", alias.Alias), slog.String("country", alias.Country))
			existing.Alias = alias.Alias
			existing.Country = alias.Country
			if err = sm.ShowAliasRepo.UpdateTx(tx, *existing); err != nil {
				sm.logger.Error("failed to update show alias", slog.String("error", err.Error()), slog.Int64("alias_id", existing.ID))
				return err
			}
		} else {
			newAlias := showAliasModel.ShowAlias{
				ShowID:  showModel.ID,
				Alias:   alias.Alias,
				Country: alias.Country,
				Source:  "tvmaze",
			}
			_, err := sm.ShowAliasRepo.CreateTx(tx, newAlias)
			if err != nil {
				sm.logger.Error("failed to create show alias", slog.String("error", err.Error()), slog.String("alias", newAlias.Alias), slog.String(
					"country",
					newAlias.Country,
				))
				return err
			}
		}
	}

	seasonMap := make(map[int]*seasonModel.Season)
	for i := range seasonsModel {
		e := &seasonsModel[i]
		seasonMap[e.Number] = e
	}

	for _, season := range seasonsDTO {

		if existing, ok := seasonMap[season.Number]; ok {
			sm.logger.Info("updating season", slog.Int64("show_id", showModel.ID), slog.Int("season_number", season.Number))

			existing.Number = season.Number
		} else {
			newSeason := seasonModel.Season{
				ShowID: showModel.ID,
				Number: season.Number,
			}
			createdSeasonID, err := sm.SeasonRepo.CreateTx(tx, newSeason)
			if err != nil {
				//TODO: Error message
				return err
			}
			seasonMap[season.Number] = &seasonModel.Season{
				ID:     createdSeasonID,
				ShowID: showModel.ID,
				Number: season.Number,
			}
		}
	}

	episodeMap := make(map[string]*episodeModel.Episode)
	for i := range episodesModel {
		e := &episodesModel[i]
		key := fmt.Sprintf("%d:%d", e.Season, e.Number)
		episodeMap[key] = e
	}

	for _, episode := range episodes {
		key := fmt.Sprintf("%d:%d", episode.Season, episode.Number)
		if existing, ok := episodeMap[key]; ok {
			sm.logger.Info("updating episode", slog.Int64("show_id", showModel.ID), slog.Int("season", episode.Season), slog.String("episode_name", episode.Name))
			existing.Name = episode.Name
			existing.Summary = episode.Summary
			existing.Type = episode.Type

			// Converting string time to int64(unix)
			dtoTime, err := utils.TimeStringToInt64(episode.AirStamp)
			if err != nil {
				return err
			}
			existing.AirStamp = dtoTime

			if err := sm.EpisodeRepo.UpdateTx(tx, *existing); err != nil {
				return err
			}
		} else {
			season := seasonMap[episode.Season]
			if season == nil {
				sm.logger.Error("missing season for episode", slog.Int("season_number", episode.Season))
				return fmt.Errorf("season %d not found when creating episode", episode.Season)
			}

			dtoTime, err := utils.TimeStringToInt64(episode.AirStamp)
			if err != nil {
				return err
			}

			newEpisode := episodeModel.Episode{
				ShowID:   showModel.ID,
				SeasonID: season.ID,
				Name:     episode.Name,
				Summary:  episode.Summary,
				Type:     episode.Type,
				Number:   episode.Number,
				Season:   episode.Season,
				AirStamp: dtoTime,
				Tracking: model.TrackingWanted,
			}

			_, err = sm.EpisodeRepo.CreateTx(tx, newEpisode)
			if err != nil {
				//TODO: Error message
				return err
			}
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}
