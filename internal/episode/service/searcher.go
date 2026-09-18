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

type EpisodeSearcherInterface interface {
	SearchEpisodeByQuery(query string) ([]schema.SearchResponse, error)
	SearchEpisodeAliasesById(episode episodeModel.Episode, show showModel.Show, db *sqlx.DB, profile *filter.Profile, ctx filter.Context) ([]schema.SearchResponse, error)
}

type EpisodeSearcher struct{}

func (s *EpisodeSearcher) SearchEpisodeByQuery(query string) ([]schema.SearchResponse, error) {
	logger := config.GetLogger()
	prowlarrIndexerService := service.NewProwlarrIndexerService(logger)

	var indexers []schema.IndexerResponse
	err := prowlarrIndexerService.GetIndexers(&indexers)
	if err != nil {
		return nil, err
	}
	var indexerIds []int
	for _, indexer := range indexers {
		if indexer.Enabled {
			indexerIds = append(indexerIds, indexer.Id)
		}
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

func (s *EpisodeSearcher) SearchEpisodeAliasesById(episode episodeModel.Episode, show showModel.Show, db *sqlx.DB, profile *filter.Profile, ctx filter.Context) ([]schema.SearchResponse, error) {
	logger := config.GetLogger()
	const cooldownDuration = 5 * time.Minute

	patterns := filterService.SearchPatterns(profile, ctx.ShowType)

	var allResponses []schema.SearchResponse

	for _, pattern := range patterns {
		for _, name := range ctx.AllNames() {
			query, err := filter.ExpandQuery(pattern, ctx, name)
			if err != nil {
				logger.Error(
					"Failed to expand search pattern",
					slog.String("pattern", pattern),
					slog.String("error", err.Error()),
				)
				continue
			}
			if strings.TrimSpace(query) == "" {
				continue
			}

			termKey := strings.ToLower(query)

			if val, ok := config.ProwlarrCooldownCache.Load(termKey); ok {
				lastTime := val.(time.Time)
				if time.Since(lastTime) < cooldownDuration {
					logger.Info("Skipping search (cooldown)", slog.String("term", termKey))
					continue
				}
			}
			config.ProwlarrCooldownCache.Store(termKey, time.Now())

			tmpResp, err := s.SearchEpisodeByQuery(query)
			if err != nil {
				logger.Error(
					"Search failed",
					slog.String("query", query),
					slog.String("error", err.Error()),
				)
				continue
			}

			if len(tmpResp) <= 0 {
				continue
			}

			allResponses = append(allResponses, tmpResp...)

			if s.hasGoodQuality(tmpResp, profile, ctx) {
				logger.Info("Good quality torrent found! Stopping extra searches", slog.String("used_name", name))
				return allResponses, nil
			}
		}
	}

	return allResponses, nil
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
