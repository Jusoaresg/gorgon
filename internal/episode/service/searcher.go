package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
	"github.com/jusoaresg/gorgon/external/prowlarr/service"
	"github.com/jusoaresg/gorgon/internal/filter"
	filterService "github.com/jusoaresg/gorgon/internal/filter/service"
	showModel "github.com/jusoaresg/gorgon/internal/show/model"
	"github.com/jusoaresg/gorgon/utils"
)

var (
	indexerCacheMu      sync.RWMutex
	cachedIndexerIDs    []int
	cachedIndexerExpiry time.Time
	indexerCacheTTL     = 5 * time.Minute
)

type SearchOptions struct {
	SkipCooldown bool
	StopOnGood   bool
	Context      context.Context
}

type EpisodeSearcherInterface interface {
	FetchRssFeed(limit int) ([]schema.SearchResponse, error)
	SearchEpisodeByQuery(query string) ([]schema.SearchResponse, error)
	SearchEpisodeAliasesById(episode episodeModel.Episode, show showModel.Show, db *sqlx.DB, profile *filter.Profile, ctx filter.Context) ([]schema.SearchResponse, error)
	SearchAliasForEpisode(episode episodeModel.Episode, show showModel.Show, alias string, db *sqlx.DB, opts SearchOptions) ([]EvaluatedResult, error)
	SearchEpisodeAliases(episode episodeModel.Episode, show showModel.Show, db *sqlx.DB, opts SearchOptions) ([]EvaluatedResult, error)
}

type EpisodeSearcher struct{}

func (s *EpisodeSearcher) getEnabledIndexerIDs(logger *slog.Logger) ([]int, error) {
	indexerCacheMu.RLock()
	if time.Now().Before(cachedIndexerExpiry) && len(cachedIndexerIDs) > 0 {
		ids := make([]int, len(cachedIndexerIDs))
		copy(ids, cachedIndexerIDs)
		indexerCacheMu.RUnlock()
		return ids, nil
	}
	indexerCacheMu.RUnlock()

	indexerCacheMu.Lock()
	defer indexerCacheMu.Unlock()

	if time.Now().Before(cachedIndexerExpiry) && len(cachedIndexerIDs) > 0 {
		ids := make([]int, len(cachedIndexerIDs))
		copy(ids, cachedIndexerIDs)
		return ids, nil
	}

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

	cachedIndexerIDs = indexerIds
	cachedIndexerExpiry = time.Now().Add(indexerCacheTTL)

	ids := make([]int, len(indexerIds))
	copy(ids, indexerIds)
	return ids, nil
}

func (s *EpisodeSearcher) FetchRssFeed(limit int) ([]schema.SearchResponse, error) {
	logger := config.GetLogger()
	indexerIds, err := s.getEnabledIndexerIDs(logger)
	if err != nil {
		return nil, err
	}

	prowlarrService, err := service.NewProwlarrSearchService(logger)
	if err != nil {
		return nil, err
	}

	var response []schema.SearchResponse
	if err := prowlarrService.FetchRssFeed(limit, &response, indexerIds...); err != nil {
		return nil, err
	}
	return response, nil
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
	if normAlias := utils.NormalizeTitle(alias); normAlias != "" && normAlias != ctx.Show {
		ctx.Aliases = append(ctx.Aliases, normAlias)
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

	searchCtx := opts.Context
	if searchCtx == nil {
		searchCtx = context.Background()
	}

	patterns := filterService.SearchPatterns(profile, settings.ShowType)

	var queries []string
	seenQueries := make(map[string]struct{})
	for _, pattern := range patterns {
		q, err := filter.ExpandQuery(pattern, ctx, alias)
		if err == nil {
			q = strings.TrimSpace(q)
			if q != "" {
				if _, seen := seenQueries[q]; !seen {
					seenQueries[q] = struct{}{}
					queries = append(queries, q)
				}
			}
		}
	}
	if len(queries) == 0 {
		queries = []string{alias}
	}

	const cooldownDuration = 5 * time.Minute
	var activeQueries []string
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
		activeQueries = append(activeQueries, query)
	}

	seen := make(map[string]struct{})
	var rawResults []schema.SearchResponse

	if len(activeQueries) == 1 {
		searchKey := schema.SearchByTypeRequest{
			Query: activeQueries[0],
			Type:  "search",
		}
		var res []schema.SearchResponse
		if err := searchService.SearchByTypeWithContext(searchCtx, &searchKey, &res, indexerIds...); err != nil {
			if !errors.Is(err, context.Canceled) {
				logger.Error("error searching prowlarr",
					slog.String("query", activeQueries[0]),
					slog.String("error", err.Error()),
				)
			}
		} else {
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
	} else if len(activeQueries) > 1 {
		type queryResult struct {
			responses []schema.SearchResponse
			err       error
		}

		resultsChan := make(chan queryResult, len(activeQueries))
		var wg sync.WaitGroup

		for _, query := range activeQueries {
			wg.Add(1)
			go func(q string) {
				defer wg.Done()

				if searchCtx.Err() != nil {
					resultsChan <- queryResult{err: searchCtx.Err()}
					return
				}

				searchKey := schema.SearchByTypeRequest{
					Query: q,
					Type:  "search",
				}

				var res []schema.SearchResponse
				if err := searchService.SearchByTypeWithContext(searchCtx, &searchKey, &res, indexerIds...); err != nil {
					if !errors.Is(err, context.Canceled) {
						logger.Error("error searching prowlarr",
							slog.String("query", q),
							slog.String("error", err.Error()),
						)
					}
					resultsChan <- queryResult{err: err}
					return
				}

				resultsChan <- queryResult{responses: res}
			}(query)
		}

		wg.Wait()
		close(resultsChan)

		if searchCtx.Err() != nil {
			return nil, searchCtx.Err()
		}

		for qr := range resultsChan {
			for _, r := range qr.responses {
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

