package service

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	seasonModel "github.com/jusoaresg/gorgon/internal/season/model"
	showModel "github.com/jusoaresg/gorgon/internal/show/model"
	showAliasModel "github.com/jusoaresg/gorgon/internal/show_aliases/model"

	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	episodeTorrentModel "github.com/jusoaresg/gorgon/internal/episode_torrent/model"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	showAliasRepository "github.com/jusoaresg/gorgon/internal/show_aliases/repository"
	showSettingsModel "github.com/jusoaresg/gorgon/internal/show_settings/model"
	showSettingsRepository "github.com/jusoaresg/gorgon/internal/show_settings/repository"
)

type ShowAggregatorService struct {
	ShowRepo           showRepository.ShowRepositoryInterface
	ShowAliasesRepo    showAliasRepository.ShowAliasesRepositoryInterface
	EpisodeRepo        episodeRepository.EpisodeRepositoryInterface
	EpisodeTorrentRepo episodeTorrentRepository.EpisodeTorrentRepositoryInterface
	SeasonRepo         seasonRepository.SeasonRepositoryInterface
	ShowSettingsRepo   showSettingsRepository.ShowSettingsRepositoryInterface
}

type AggregatedShow struct {
	Show        showModel.Show
	ShowAliases []showAliasModel.ShowAlias
	Seasons     []seasonModel.Season
	Episodes    []episodeModel.Episode
	Torrents    map[int64]episodeTorrentModel.EpisodeTorrent
	Settings    showSettingsModel.ShowSettings
}

func NewShowAggregatorService(
	showRepo showRepository.ShowRepositoryInterface,
	showAliasRepo showAliasRepository.ShowAliasesRepositoryInterface,
	episodeRepo episodeRepository.EpisodeRepositoryInterface,
	episodeTorrentRepo episodeTorrentRepository.EpisodeTorrentRepositoryInterface,
	seasonRepo seasonRepository.SeasonRepositoryInterface,
	showSettingsRepo showSettingsRepository.ShowSettingsRepositoryInterface,
) *ShowAggregatorService {
	return &ShowAggregatorService{
		ShowRepo:           showRepo,
		ShowAliasesRepo:    showAliasRepo,
		EpisodeRepo:        episodeRepo,
		EpisodeTorrentRepo: episodeTorrentRepo,
		SeasonRepo:         seasonRepo,
		ShowSettingsRepo:   showSettingsRepo,
	}
}

func NewShowAggregatorServiceWithDb(db *sqlx.DB) *ShowAggregatorService {
	aliasRepo := showAliasRepository.NewShowAliasesRepository(db)
	settingsRepo := showSettingsRepository.NewShowSettingsRepository(db)
	return &ShowAggregatorService{
		ShowRepo:           showRepository.NewShowRepository(db),
		ShowAliasesRepo:    &aliasRepo,
		EpisodeRepo:        episodeRepository.NewEpisodeRepository(db),
		EpisodeTorrentRepo: episodeTorrentRepository.NewEpisodeTorrentRepository(db),
		SeasonRepo:         seasonRepository.NewSeasonRepository(db),
		ShowSettingsRepo:   &settingsRepo,
	}
}

func (s *ShowAggregatorService) torrentsByEpisode(episodes []episodeModel.Episode) (map[int64]episodeTorrentModel.EpisodeTorrent, error) {
	episodeIDs := make([]int64, 0, len(episodes))
	for _, ep := range episodes {
		episodeIDs = append(episodeIDs, ep.ID)
	}

	torrents, err := s.EpisodeTorrentRepo.ListByEpisodeIDs(episodeIDs)
	if err != nil {
		return nil, err
	}

	byEpisode := make(map[int64]episodeTorrentModel.EpisodeTorrent, len(torrents))
	for _, t := range torrents {
		byEpisode[t.EpisodeId] = t
	}

	return byEpisode, nil
}

func (s *ShowAggregatorService) GetShowWithRelationsByTvMazeId(tvMazeID int64) (AggregatedShow, error) {

	show, err := s.ShowRepo.GetByTvMazeID(tvMazeID)
	if err != nil {
		return AggregatedShow{}, fmt.Errorf("failed to get show by tvmazeID: %w", err)
	}

	showAliases, err := s.ShowAliasesRepo.ListByShowID(show.ID)
	if err != nil {
		return AggregatedShow{}, fmt.Errorf("failed to get show aliases: %w", err)
	}

	season, err := s.SeasonRepo.ListByShowId(show.ID)
	if err != nil {
		return AggregatedShow{}, fmt.Errorf("Failed to get season by showID: %w", err)
	}

	episode, err := s.EpisodeRepo.ListByShowID(show.ID)
	if err != nil {
		return AggregatedShow{}, fmt.Errorf("Failed to get episode by showID: %w", err)
	}

	torrents, err := s.torrentsByEpisode(episode)
	if err != nil {
		return AggregatedShow{}, fmt.Errorf("Failed to get episode torrents: %w", err)
	}

	var settings showSettingsModel.ShowSettings
	if s.ShowSettingsRepo != nil {
		settings, _ = s.ShowSettingsRepo.GetByShowID(show.ID)
	}
	if settings.ShowType == "" {
		settings.ShowType = showSettingsModel.ShowTypeStandard
	}

	return AggregatedShow{
		Show:        show,
		ShowAliases: showAliases,
		Seasons:     season,
		Episodes:    episode,
		Torrents:    torrents,
		Settings:    settings,
	}, nil
}

func (s *ShowAggregatorService) GetShowWithRelationsById(id int64) (AggregatedShow, error) {

	show, err := s.ShowRepo.GetById(id)
	if err != nil {
		return AggregatedShow{}, fmt.Errorf("failed to get show by tvmazeID: %w", err)
	}

	showAliases, err := s.ShowAliasesRepo.ListByShowID(show.ID)
	if err != nil {
		return AggregatedShow{}, fmt.Errorf("failed to get show aliases: %w", err)
	}

	season, err := s.SeasonRepo.ListByShowId(show.ID)
	if err != nil {
		return AggregatedShow{}, fmt.Errorf("Failed to get season by showID: %w", err)
	}

	episode, err := s.EpisodeRepo.ListByShowID(show.ID)
	if err != nil {
		return AggregatedShow{}, fmt.Errorf("Failed to get episode by showID: %w", err)
	}

	torrents, err := s.torrentsByEpisode(episode)
	if err != nil {
		return AggregatedShow{}, fmt.Errorf("Failed to get episode torrents: %w", err)
	}

	var settings showSettingsModel.ShowSettings
	if s.ShowSettingsRepo != nil {
		settings, _ = s.ShowSettingsRepo.GetByShowID(show.ID)
	}
	if settings.ShowType == "" {
		settings.ShowType = showSettingsModel.ShowTypeStandard
	}

	return AggregatedShow{
		Show:        show,
		ShowAliases: showAliases,
		Seasons:     season,
		Episodes:    episode,
		Torrents:    torrents,
		Settings:    settings,
	}, nil
}

func (s *ShowAggregatorService) ListFullShows() ([]AggregatedShow, error) {
	shows, err := s.ShowRepo.List()
	if err != nil {
		return nil, err
	}

	var aggregated []AggregatedShow

	for _, show := range shows {
		aliases, err := s.ShowAliasesRepo.ListByShowID(show.ID)
		if err != nil {
			return nil, err
		}

		seasons, err := s.SeasonRepo.ListByShowId(show.ID)
		if err != nil {
			return nil, err
		}

		episodes, err := s.EpisodeRepo.ListByShowID(show.ID)
		if err != nil {
			return nil, err
		}

		torrents, err := s.torrentsByEpisode(episodes)
		if err != nil {
			return nil, err
		}

		aggregated = append(aggregated, AggregatedShow{
			Show:        show,
			ShowAliases: aliases,
			Seasons:     seasons,
			Episodes:    episodes,
			Torrents:    torrents,
		})
	}

	return aggregated, nil
}

func (s *ShowAggregatorService) ListFullShowsFiltered(search, status string) ([]AggregatedShow, error) {
	shows, err := s.ShowRepo.ListFiltered(search, status)
	if err != nil {
		return nil, err
	}

	var aggregated []AggregatedShow

	for _, show := range shows {
		aliases, err := s.ShowAliasesRepo.ListByShowID(show.ID)
		if err != nil {
			return nil, err
		}

		seasons, err := s.SeasonRepo.ListByShowId(show.ID)
		if err != nil {
			return nil, err
		}

		episodes, err := s.EpisodeRepo.ListByShowID(show.ID)
		if err != nil {
			return nil, err
		}

		torrents, err := s.torrentsByEpisode(episodes)
		if err != nil {
			return nil, err
		}

		aggregated = append(aggregated, AggregatedShow{
			Show:        show,
			ShowAliases: aliases,
			Seasons:     seasons,
			Episodes:    episodes,
			Torrents:    torrents,
		})
	}

	return aggregated, nil
}
