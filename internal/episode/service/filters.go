package service

import (
	"log/slog"
	"sort"

	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	"github.com/jusoaresg/gorgon/internal/filter"
)

type ScoredResponse struct {
	Response schema.SearchResponse
	Score    int
}

type EvaluatedResult struct {
	schema.SearchResponse
	EpisodeMatched  bool
	PassedFilter    bool
	Score           int
	RejectionReason string
	Metadata        filter.ReleaseMetadata
}

// FilterAndScoreResponses applies episode matching and the profile gates to every response,
// sorting candidates by intrinsic quality with swarm health (seeders) used for tie-breaking.
func FilterAndScoreResponses(responses []schema.SearchResponse, profile *filter.Profile, ctx filter.Context) []schema.SearchResponse {
	logger := config.GetLogger()

	var scored []ScoredResponse
	for _, response := range responses {
		name := releaseName(response)
		if !filter.MatchEpisode(name, ctx.Season, ctx.Episode, ctx.ShowType) {
			logger.Debug(
				"Release rejected by episode mismatch",
				slog.String("filename", name),
				slog.Int("season", ctx.Season),
				slog.Int("episode", ctx.Episode),
				slog.String("show_type", ctx.ShowType),
			)
			continue
		}

		result := filter.Evaluate(profile, ctx, name)
		if !result.Passed {
			logger.Debug(
				"Release rejected by filter",
				slog.String("filename", name),
				slog.String("reason", result.RejectedReason),
			)
			continue
		}

		// Rank order considers preferred pattern score + resolution + swarm health bonus
		rankScore := result.PreferredScore + getQualityScore(response) + getHealthScore(response)

		scored = append(scored, ScoredResponse{
			Response: response,
			Score:    rankScore,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	filtered := make([]schema.SearchResponse, 0, len(scored))
	for _, s := range scored {
		filtered = append(filtered, s.Response)
	}
	return filtered
}

// IsGoodResponse reports whether a response cleared episode match and profile gates and
// achieved a high quality score to halt secondary alias queries.
func IsGoodResponse(response schema.SearchResponse, profile *filter.Profile, ctx filter.Context) bool {
	const goodScore = 60
	name := releaseName(response)

	if !filter.MatchEpisode(name, ctx.Season, ctx.Episode, ctx.ShowType) {
		return false
	}

	result := filter.Evaluate(profile, ctx, name)
	if !result.Passed {
		return false
	}

	return result.PreferredScore+getQualityScore(response)+getHealthScore(response) >= goodScore
}

// EvaluateResponse computes episode matching, filter profile compliance, release metadata, and canonical score for a single release.
func EvaluateResponse(response schema.SearchResponse, profile *filter.Profile, ctx filter.Context) EvaluatedResult {
	name := releaseName(response)
	epMatch := filter.MatchEpisode(name, ctx.Season, ctx.Episode, ctx.ShowType)
	eval := filter.Evaluate(profile, ctx, name)
	score := CalculateScore(response, profile, ctx)
	meta := filter.ParseReleaseMetadata(name)

	item := EvaluatedResult{
		SearchResponse: response,
		EpisodeMatched: epMatch,
		Score:          score,
		Metadata:       meta,
	}

	if !epMatch {
		item.PassedFilter = false
		item.RejectionReason = "Episode mismatch"
	} else if !eval.Passed {
		item.PassedFilter = false
		item.RejectionReason = eval.RejectedReason
	} else {
		item.PassedFilter = true
	}

	return item
}

// EvaluateAndSortResponses evaluates all candidate responses and sorts them:
// passed first, then episode matched, then by canonical score descending, and seeders as tie-breaker.
func EvaluateAndSortResponses(responses []schema.SearchResponse, profile *filter.Profile, ctx filter.Context) []EvaluatedResult {
	evaluated := make([]EvaluatedResult, 0, len(responses))
	for _, r := range responses {
		evaluated = append(evaluated, EvaluateResponse(r, profile, ctx))
	}

	sort.Slice(evaluated, func(i, j int) bool {
		if evaluated[i].PassedFilter != evaluated[j].PassedFilter {
			return evaluated[i].PassedFilter
		}
		if evaluated[i].EpisodeMatched != evaluated[j].EpisodeMatched {
			return evaluated[i].EpisodeMatched
		}
		if evaluated[i].Score != evaluated[j].Score {
			return evaluated[i].Score > evaluated[j].Score
		}
		return evaluated[i].Seeders > evaluated[j].Seeders
	})

	return evaluated
}

// CalculateScore computes the canonical intrinsic release quality score (preferred tags + resolution).
// Swarm health is intentionally excluded so ephemeral seed counts do not pollute cutoff thresholds.
func CalculateScore(response schema.SearchResponse, profile *filter.Profile, ctx filter.Context) int {
	name := releaseName(response)
	result := filter.Evaluate(profile, ctx, name)
	return result.PreferredScore + getQualityScore(response)
}

// ShouldUpgrade determines whether a candidate release qualifies as an upgrade over an existing episode.
func ShouldUpgrade(tracking string, profile *filter.Profile, currentScore, candidateScore int) bool {
	isExisting := tracking == model.TrackingDownloaded || tracking == model.TrackingSnatched
	if !isExisting {
		return false
	}
	if profile == nil || profile.UpgradeUntilScore <= 0 {
		return false
	}
	if currentScore >= profile.UpgradeUntilScore {
		return false
	}
	return candidateScore > currentScore
}
