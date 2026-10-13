package service

import (
	"log/slog"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
	episodeEvents "github.com/jusoaresg/gorgon/internal/episode/events"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	filterService "github.com/jusoaresg/gorgon/internal/filter/service"
	showModel "github.com/jusoaresg/gorgon/internal/show/model"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
)

const (
	SearchResultSnatched  = "snatched"
	SearchResultNoResults = "noResults"
	SearchResultNotAired  = "notAired"
	SearchResultError     = "error"
)

type EpisodeSearchInterface interface {
	ProcessSingleEpisode(episodeID int) error
	ProcessSeasonEpisodes(seasonID int)
	ProcessEpisodeReleases(episode model.Episode, show showModel.Show, responses []schema.SearchResponse, emitEvents bool) (bool, error)
}

type EpisodeSearchService struct {
	db          *sqlx.DB
	logger      *slog.Logger
	Searcher    EpisodeSearcherInterface
	Downloader  EpisodeDownloaderInterface
	EpisodeRepo episodeRepository.EpisodeRepositoryInterface
	ShowRepo    showRepository.ShowRepositoryInterface
}

func NewEpisodeSearchService(
	db *sqlx.DB,
	logger *slog.Logger,
	searcher EpisodeSearcherInterface,
	downloader EpisodeDownloaderInterface,
	episodeRepo episodeRepository.EpisodeRepositoryInterface,
	showRepo showRepository.ShowRepositoryInterface,
) *EpisodeSearchService {
	return &EpisodeSearchService{
		db:          db,
		logger:      logger,
		Searcher:    searcher,
		Downloader:  downloader,
		EpisodeRepo: episodeRepo,
		ShowRepo:    showRepo,
	}
}

func (s *EpisodeSearchService) ProcessSingleEpisode(episodeID int) error {
	episode, err := s.EpisodeRepo.GetByID(int64(episodeID))
	if err != nil {
		s.emitSearchFinished(model.Episode{}, showModel.Show{}, SearchResultError, err.Error())
		return err
	}

	if aired := episode.HasAired(); !aired {
		s.logger.Info(
			"Episode has not aired yet",
			slog.Int64("episode_id", episode.ID),
			slog.Int64("show_id", episode.ShowID),
		)
		s.emitSearchFinished(episode, showModel.Show{}, SearchResultNotAired, "")
		return nil
	}

	show, err := s.ShowRepo.GetById(int64(episode.ShowID))
	if err != nil {
		s.emitSearchFinished(episode, showModel.Show{}, SearchResultError, err.Error())
		return err
	}

	_ = s.EpisodeRepo.UpdateLastSearchedAt(episode.ID, time.Now().Unix())

	s.logger.Info(
		"Searching if episode is available",
		slog.String("show_name", show.Name),
		slog.Int64("show_id", show.ID),
		slog.Int("episode", episode.Number),
		slog.String("episode_name", episode.Name),
		slog.String("tracking", string(episode.Tracking)),
	)

	evaluated, err := s.Searcher.SearchEpisodeAliases(episode, show, s.db, SearchOptions{
		SkipCooldown: false,
		StopOnGood:   true,
	})
	if err != nil {
		s.emitSearchFinished(episode, show, SearchResultError, err.Error())
		return err
	}

	if len(evaluated) == 0 {
		s.logger.Info("No available episode found", slog.String("show", show.Name), slog.Int("episode", episode.Number))
		s.emitSearchFinished(episode, show, SearchResultNoResults, "")
		return nil
	}

	responses := make([]schema.SearchResponse, 0, len(evaluated))
	for _, item := range evaluated {
		responses = append(responses, item.SearchResponse)
	}

	_, err = s.ProcessEpisodeReleases(episode, show, responses, true)
	return err
}

// ProcessEpisodeReleases is the unified pipeline for evaluating candidate releases against a show's filter profile,
// scoring them, checking upgrade cutoffs, and triggering either a fresh download or an upgrade.
func (s *EpisodeSearchService) ProcessEpisodeReleases(episode model.Episode, show showModel.Show, responses []schema.SearchResponse, emitEvents bool) (bool, error) {
	settings, err := filterService.ResolveSettings(s.db, show.ID)
	if err != nil {
		if emitEvents {
			s.emitSearchFinished(episode, show, SearchResultError, err.Error())
		}
		return false, err
	}

	profile, err := filterService.ResolveProfile(s.db, settings)
	if err != nil {
		if emitEvents {
			s.emitSearchFinished(episode, show, SearchResultError, err.Error())
		}
		return false, err
	}

	ctx, err := filterService.BuildContext(s.db, show, episode.Season, episode.Number, settings)
	if err != nil {
		if emitEvents {
			s.emitSearchFinished(episode, show, SearchResultError, err.Error())
		}
		return false, err
	}

	filtered := FilterAndScoreResponses(responses, profile, ctx)
	if len(filtered) == 0 {
		s.logger.Debug(
			"No matching releases after filtering",
			slog.Int64("episode_id", episode.ID),
			slog.Int64("show_id", episode.ShowID),
		)
		if emitEvents {
			s.emitSearchFinished(episode, show, SearchResultNoResults, "")
		}
		return false, nil
	}

	best := filtered[0]
	score := CalculateScore(best, profile, ctx)

	// Handle upgrades for episodes already snatched or downloaded
	if episode.Tracking == model.TrackingDownloaded || episode.Tracking == model.TrackingSnatched {
		epTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(s.db)
		existingTorrent, err := epTorrentRepo.GetByEpisodeID(episode.ID)
		existingScore := 0
		if err == nil {
			existingScore = existingTorrent.Score
		}

		if !ShouldUpgrade(episode.Tracking, profile, existingScore, score) {
			s.logger.Debug("Candidate release does not qualify for upgrade",
				slog.Int64("episode_id", episode.ID),
				slog.Int("existing_score", existingScore),
				slog.Int("candidate_score", score),
			)
			if emitEvents {
				s.emitSearchFinished(episode, show, SearchResultNoResults, "Already downloaded with sufficient quality")
			}
			return false, nil
		}

		s.logger.Info("Triggering release upgrade",
			slog.Int64("episode_id", episode.ID),
			slog.Int64("show_id", episode.ShowID),
			slog.Int("old_score", existingScore),
			slog.Int("new_score", score),
			slog.Int("cutoff", profile.UpgradeUntilScore),
			slog.String("new_release", best.Title),
		)

		if err := s.Downloader.UpgradeEpisode(episode, best, score); err != nil {
			if emitEvents {
				s.emitSearchFinished(episode, show, SearchResultError, err.Error())
			}
			return false, err
		}
		if emitEvents {
			s.emitSearchFinished(episode, show, SearchResultSnatched, "")
		}
		return true, nil
	}

	// Standard download for wanted or missing episodes
	s.logger.Info(
		"Downloading episode",
		slog.String("title", best.Title),
		slog.Int64("show_id", episode.ShowID),
		slog.Int64("episode_id", episode.ID),
		slog.Int("score", score),
	)
	if err := s.Downloader.DownloadEpisode(episode, best, score); err != nil {
		if emitEvents {
			s.emitSearchFinished(episode, show, SearchResultError, err.Error())
		}
		return false, err
	}
	if emitEvents {
		s.emitSearchFinished(episode, show, SearchResultSnatched, "")
	}
	return true, nil
}

func (s *EpisodeSearchService) emitSearchFinished(episode model.Episode, show showModel.Show, result, message string) {
	episodeEvents.EmitEpisodeSearchFinishedEvent(episode.ID, episode.Season, episode.Number, episode.Name, show.Name, result, message)
}

func (s *EpisodeSearchService) ProcessShowWantedEpisodes(showID int) {
	go func() {
		allEpisodes, err := s.EpisodeRepo.ListByShowID(int64(showID))
		if err != nil {
			return
		}

		var episodes []model.Episode
		for _, episode := range allEpisodes {
			if episode.Tracking == model.TrackingMissing ||
				episode.Tracking == model.TrackingWanted {
				episodes = append(episodes, episode)
			}
		}
		if len(episodes) <= 0 {
			return
		}

		show, err := s.ShowRepo.GetById(int64(showID))
		if err != nil {
			return
		}

		var notAiredYet []model.Episode

		var wg sync.WaitGroup
		for _, episode := range episodes {
			ep := episode

			if aired := ep.HasAired(); !aired {
				notAiredYet = append(notAiredYet, episode)
				continue
			}

			wg.Go(func() {
				if err := s.ProcessSingleEpisode(int(ep.ID)); err != nil {
					s.logger.Error(
						"Error processing episode",
						slog.Int64("episode_id", ep.ID),
						slog.Any("err", err),
					)
					return
				}
			})

		}
		wg.Wait()

		s.logger.Info(
			"Episodes not aired yet",
			slog.Any("episode_id", notAiredYet),
			slog.Int64("show_id", show.ID),
		)
	}()
}

func (s *EpisodeSearchService) ProcessSeasonEpisodes(seasonID int) {
	go func() {
		allEpisodes, err := s.EpisodeRepo.ListBySeasonID(seasonID)
		if err != nil {
			return
		}

		var wg sync.WaitGroup
		for _, episode := range allEpisodes {
			ep := episode

			if aired := ep.HasAired(); !aired {
				continue
			}

			wg.Go(func() {
				if err := s.ProcessSingleEpisode(int(ep.ID)); err != nil {
					s.logger.Error(
						"Error processing episode",
						slog.Int64("episode_id", ep.ID),
						slog.Any("err", err),
					)
					return
				}
			})
		}
		wg.Wait()
	}()
}
