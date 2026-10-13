package workers

import (
	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	episodeService "github.com/jusoaresg/gorgon/internal/episode/service"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	"github.com/jusoaresg/gorgon/internal/filter"
	filterService "github.com/jusoaresg/gorgon/internal/filter/service"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
)

type RssReleaseProcessor struct {
	db                 *sqlx.DB
	showRepo           showRepository.ShowRepositoryInterface
	seasonRepo         seasonRepository.SeasonRepositoryInterface
	episodeTorrentRepo episodeTorrentRepository.EpisodeTorrentRepositoryInterface
	downloader         episodeService.EpisodeDownloaderInterface
}

func NewRssReleaseProcessor(db *sqlx.DB) RssReleaseProcessor {
	return RssReleaseProcessor{
		db:                 db,
		showRepo:           showRepository.NewShowRepository(db),
		seasonRepo:         seasonRepository.NewSeasonRepository(db),
		episodeTorrentRepo: episodeTorrentRepository.NewEpisodeTorrentRepository(db),
		downloader:         episodeService.NewEpisodeDownloader(db),
	}
}

// RssProcessRelease delegates evaluation, scoring, upgrade checks, and downloading to EpisodeSearchService.
func (rss *RssReleaseProcessor) RssProcessRelease(ep model.Episode, responses []schema.SearchResponse) error {
	logger := config.GetLogger().WithGroup("worker").With("name", "RssProcessRelease")

	show, err := rss.showRepo.GetById(ep.ShowID)
	if err != nil {
		return err
	}

	searchSvc := episodeService.NewEpisodeSearchService(
		rss.db,
		logger,
		&episodeService.EpisodeSearcher{},
		rss.downloader,
		episodeRepository.NewEpisodeRepository(rss.db),
		rss.showRepo,
	)

	_, err = searchSvc.ProcessEpisodeReleases(ep, show, responses, false)
	return err
}

// MatchReleasesToEpisodes performs a release-first pass by grouping episodes per show,
// matching RSS items against each show's title/aliases once, and returning only episodes that have matching releases.
func (rss *RssReleaseProcessor) MatchReleasesToEpisodes(episodes []model.Episode, responses []schema.SearchResponse) []EpisodeJob {
	if len(episodes) == 0 || len(responses) == 0 {
		return nil
	}

	byShow := make(map[int64][]model.Episode)
	for _, ep := range episodes {
		byShow[ep.ShowID] = append(byShow[ep.ShowID], ep)
	}

	var jobs []EpisodeJob
	for showID, showEpisodes := range byShow {
		show, err := rss.showRepo.GetById(showID)
		if err != nil {
			continue
		}

		settings, err := filterService.ResolveSettings(rss.db, showID)
		if err != nil {
			continue
		}

		showCtx, err := filterService.BuildContext(rss.db, show, 0, 0, settings)
		if err != nil {
			continue
		}

		// Filter RSS items that match this show's title or aliases
		var showResponses []schema.SearchResponse
		for _, r := range responses {
			name := r.Filename
			if name == "" {
				name = r.Title
			}
			if episodeService.MatchShowTitle(name, showCtx) {
				showResponses = append(showResponses, r)
			}
		}

		if len(showResponses) == 0 {
			continue
		}

		// Match specific episodes within the show
		for _, ep := range showEpisodes {
			var epResponses []schema.SearchResponse
			for _, r := range showResponses {
				name := r.Filename
				if name == "" {
					name = r.Title
				}
				if filter.MatchEpisode(name, ep.Season, ep.Number, showCtx.ShowType) {
					epResponses = append(epResponses, r)
				}
			}
			if len(epResponses) > 0 {
				jobs = append(jobs, EpisodeJob{
					Episode:   ep,
					Responses: epResponses,
				})
			}
		}
	}

	return jobs
}
