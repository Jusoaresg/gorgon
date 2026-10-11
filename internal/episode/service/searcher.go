package service

import (
	"log/slog"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
	"github.com/jusoaresg/gorgon/external/prowlarr/service"
	"github.com/jusoaresg/gorgon/internal/filter"
	filterService "github.com/jusoaresg/gorgon/internal/filter/service"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	showModel "github.com/jusoaresg/gorgon/internal/show/model"
)

type SearchOptions struct {
	SkipCooldown bool
	StopOnGood   bool
}

type EpisodeSearcherInterface interface {
	SearchEpisodeByQuery(query string) ([]schema.SearchResponse, error)
	SearchEpisodeAliasesById(episode episodeModel.Episode, show showModel.Show, db *sqlx.DB, profile *filter.Profile, ctx filter.Context) ([]schema.SearchResponse, error)
	SearchAliasForEpisode(episode episodeModel.Episode, show showModel.Show, alias string, db *sqlx.DB, opts SearchOptions) ([]EvaluatedResult, error)
	SearchEpisodeAliases(episode episodeModel.Episode, show showModel.Show, db *sqlx.DB, opts SearchOptions) ([]EvaluatedResult, error)
}

type EpisodeSearcher struct{}

func (s *EpisodeSearcher) getEnabledIndexerIDs(logger *slog.Logger) ([]int, error) {
	prowlarrIndexerService := service.NewProwlarrIndexerService(logger)
	var indexers []schema.IndexerResponse
	if err := prowlarrIndexerService.GetIndexers(&indexers); err != nil {
		return nil, err
	}
	var indexerIds []int
	for _, indexer := range indexers {
		if indexer.Enabled {
			indexerIds = append(indexerIds, indexer.Id)
		}
	}
	return indexerIds, nil
}

func (s *EpisodeSearcher) SearchEpisodeByQuery(query string) ([]schema.SearchResponse, error) {
	logger := config.GetLogger()
	indexerIds, err := s.getEnabledIndexerIDs(logger)
	if err != nil {
		return nil, err
	}

	prowlarrService, err := service.NewProwlarrSearchService(logger)
	if err != nil {
		return nil, err
	}

	request := schema.SearchRequest{
		Query: query,
	}

	var response []schema.SearchResponse
	err = prowlarrService.Search(&request, &response, indexerIds...)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// SearchAliasForEpisode searches Prowlarr for a single alias/title using the show's filter profile and patterns,
// then returns all releases evaluated and sorted.
func (s *EpisodeSearcher) SearchAliasForEpisode(episode episodeModel.Episode, show showModel.Show, alias string, db *sqlx.DB, opts SearchOptions) ([]EvaluatedResult, error) {
	logger := config.GetLogger()

	settings, err := filterService.ResolveSettings(db, show.ID)
	if err != nil {
		return nil, err
	}

	profile, err := filterService.ResolveProfile(db, settings)
	if err != nil {
		return nil, err
	}

	ctx, err := filterService.BuildContext(db, show, episode.Season, episode.Number, settings)
	if err != nil {
		return nil, err
	}

	indexerIds, err := s.getEnabledIndexerIDs(logger)
	if err != nil {
		logger.Error("error fetching prowlarr indexers", slog.String("error", err.Error()))
		return nil, err
	}

	var searchService *service.ProwlarrSearchService
	if opts.SkipCooldown {
		searchService, err = service.NewInteractiveProwlarrSearchService(logger)
	} else {
		searchService, err = service.NewProwlarrSearchService(logger)
	}
	if err != nil {
		logger.Error("error initializing prowlarr service", slog.String("error", err.Error()))
		return nil, err
	}

	patterns := filterService.SearchPatterns(profile, settings.ShowType)

	var queries []string
	for _, pattern := range patterns {
		q, err := filter.ExpandQuery(pattern, ctx, alias)
		if err == nil && strings.TrimSpace(q) != "" {
			queries = append(queries, q)
		}
	}
	if len(queries) == 0 {
		queries = []string{alias}
	}

	const cooldownDuration = 5 * time.Minute
	seen := make(map[string]struct{})
	var rawResults []schema.SearchResponse

	for _, query := range queries {
		termKey := strings.ToLower(query)
		if !opts.SkipCooldown {
			if val, ok := config.ProwlarrCooldownCache.Load(termKey); ok {
				lastTime := val.(time.Time)
				if time.Since(lastTime) < cooldownDuration {
					logger.Info("Skipping search (cooldown)", slog.String("term", termKey))
					continue
				}
			}
			config.ProwlarrCooldownCache.Store(termKey, time.Now())
		}

		searchKey := schema.SearchByTypeRequest{
			Query: query,
			Type:  "search",
		}

		var res []schema.SearchResponse
		if err := searchService.SearchByType(&searchKey, &res, indexerIds...); err != nil {
			logger.Error("error searching prowlarr",
				slog.String("query", query),
				slog.String("error", err.Error()),
			)
			continue
		}

		for _, r := range res {
			idKey := r.InfoHash
			if idKey == "" {
				idKey = r.Guid
			}
			if idKey == "" {
				idKey = r.Filename
			}
			if _, exists := seen[idKey]; !exists {
				seen[idKey] = struct{}{}
				rawResults = append(rawResults, r)
			}
		}
	}

	return EvaluateAndSortResponses(rawResults, profile, ctx), nil
}

// SearchEpisodeAliases searches across all aliases and patterns for an episode.
func (s *EpisodeSearcher) SearchEpisodeAliases(episode episodeModel.Episode, show showModel.Show, db *sqlx.DB, opts SearchOptions) ([]EvaluatedResult, error) {
	logger := config.GetLogger()

	settings, err := filterService.ResolveSettings(db, show.ID)
	if err != nil {
		return nil, err
	}

	profile, err := filterService.ResolveProfile(db, settings)
	if err != nil {
		return nil, err
	}

	ctx, err := filterService.BuildContext(db, show, episode.Season, episode.Number, settings)
	if err != nil {
		return nil, err
	}

	names := ctx.AllNames()
	if len(names) == 0 {
		names = []string{show.Name}
	}

	seen := make(map[string]struct{})
	var allEvaluated []EvaluatedResult

	for _, name := range names {
		evaluated, err := s.SearchAliasForEpisode(episode, show, name, db, opts)
		if err != nil {
			logger.Error("Search alias failed", slog.String("name", name), slog.String("error", err.Error()))
			continue
		}

		for _, item := range evaluated {
			idKey := item.InfoHash
			if idKey == "" {
				idKey = item.Guid
			}
			if idKey == "" {
				idKey = item.Filename
			}
			if _, exists := seen[idKey]; !exists {
				seen[idKey] = struct{}{}
				allEvaluated = append(allEvaluated, item)
			}
		}

		if opts.StopOnGood {
			for _, item := range evaluated {
				if item.PassedFilter && IsGoodResponse(item.SearchResponse, profile, ctx) {
					logger.Info("Good quality torrent found! Stopping extra searches", slog.String("used_name", name))
					return allEvaluated, nil
				}
			}
		}
	}

	return allEvaluated, nil
}

func (s *EpisodeSearcher) SearchEpisodeAliasesById(episode episodeModel.Episode, show showModel.Show, db *sqlx.DB, profile *filter.Profile, ctx filter.Context) ([]schema.SearchResponse, error) {
	evaluated, err := s.SearchEpisodeAliases(episode, show, db, SearchOptions{
		SkipCooldown: false,
		StopOnGood:   true,
	})
	if err != nil {
		return nil, err
	}

	var responses []schema.SearchResponse
	for _, item := range evaluated {
		responses = append(responses, item.SearchResponse)
	}
	return responses, nil
}

func (s *EpisodeSearcher) hasGoodQuality(results []schema.SearchResponse, profile *filter.Profile, ctx filter.Context) bool {
	for _, response := range results {
		if IsGoodResponse(response, profile, ctx) {
			return true
		}
	}
	return false
}

var _ EpisodeSearcherInterface = (*EpisodeSearcher)(nil)

