package service

import (
	"testing"

	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
	"github.com/jusoaresg/gorgon/internal/filter"
	"github.com/stretchr/testify/assert"
)

func filterTestContext() filter.Context {
	return filter.Context{
		Show:    "dragon ball",
		Season:  1,
		Episode: 4,
	}
}

func filterTestProfile() *filter.Profile {
	return &filter.Profile{
		Required: []string{"{alias} S{season:00}E{episode:00}", "multisub"},
		Rejected: []string{"hdtv"},
		Preferred: []filter.PreferredPattern{
			{Pattern: "web", Score: 30},
			{Pattern: "1080p", Score: 20},
		},
	}
}

func TestFilterAndScoreResponses_KeepsOnlyPassingSortedByScore(t *testing.T) {
	responses := []schema.SearchResponse{
		{Filename: "Dragon Ball S01E04 1080p MULTISUB WEB", Seeders: 10},
		{Filename: "Dragon Ball S01E04 HDTV MULTISUB", Seeders: 20},
		{Filename: "Naruto S01E04 MULTISUB", Seeders: 30},
		{Filename: "Dragon Ball S01E04 720p MULTISUB", Seeders: 5},
		{Filename: "Dragon Ball S01E04 MULTISUB", Seeders: 2},
	}

	result := FilterAndScoreResponses(responses, filterTestProfile(), filterTestContext())

	assert.Len(t, result, 3)
	assert.Equal(t, "Dragon Ball S01E04 1080p MULTISUB WEB", result[0].Filename)
	assert.Equal(t, "Dragon Ball S01E04 720p MULTISUB", result[1].Filename)
	assert.Equal(t, "Dragon Ball S01E04 MULTISUB", result[2].Filename)
}

func TestFilterAndScoreResponses_NilProfileKeepsAllMatching(t *testing.T) {
	responses := []schema.SearchResponse{
		{Filename: "Dragon Ball S01E04 720p"},
		{Filename: "Dragon Ball 1x04 1080p"},
	}

	result := FilterAndScoreResponses(responses, nil, filterTestContext())
	assert.Len(t, result, 2)
}

func TestFilterAndScoreResponses_RejectsWrongEpisode(t *testing.T) {
	responses := []schema.SearchResponse{
		{Filename: "Dragon Ball S01E05 1080p MULTISUB WEB", Seeders: 100}, // Ep 5 with 100 seeders
		{Filename: "Dragon Ball S01E04 1080p MULTISUB WEB", Seeders: 10},  // Ep 4 with 10 seeders
	}

	result := FilterAndScoreResponses(responses, filterTestProfile(), filterTestContext())
	assert.Len(t, result, 1)
	assert.Equal(t, "Dragon Ball S01E04 1080p MULTISUB WEB", result[0].Filename)
}


func TestIsGoodResponse(t *testing.T) {
	ctx := filterTestContext()
	profile := filterTestProfile()

	good := schema.SearchResponse{Filename: "Dragon Ball S01E04 1080p MULTISUB WEB", Seeders: 10}
	assert.True(t, IsGoodResponse(good, profile, ctx))

	mediocre := schema.SearchResponse{Filename: "Dragon Ball S01E04 720p MULTISUB", Seeders: 5}
	assert.False(t, IsGoodResponse(mediocre, profile, ctx))

	rejected := schema.SearchResponse{Filename: "Dragon Ball S01E04 HDTV MULTISUB", Seeders: 50}
	assert.False(t, IsGoodResponse(rejected, profile, ctx))
}

func TestEvaluateAndSortResponses(t *testing.T) {
	ctx := filterTestContext()
	profile := filterTestProfile()

	responses := []schema.SearchResponse{
		{Filename: "Dragon Ball S01E04 1080p MULTISUB WEB", Seeders: 10},
		{Filename: "Dragon Ball S01E04 HDTV MULTISUB", Seeders: 20},
		{Filename: "Dragon Ball S02E04 MULTISUB", Seeders: 30},
	}

	evaluated := EvaluateAndSortResponses(responses, profile, ctx)
	assert.Len(t, evaluated, 3)

	// First should be the passing one with highest score
	assert.True(t, evaluated[0].PassedFilter)
	assert.True(t, evaluated[0].EpisodeMatched)
	assert.Equal(t, "Dragon Ball S01E04 1080p MULTISUB WEB", evaluated[0].Filename)
	assert.Equal(t, "1080P", evaluated[0].Metadata.Resolution)

	// HDTV matches episode but rejected by filter
	var hdtv *EvaluatedResult
	for i := range evaluated {
		if evaluated[i].Filename == "Dragon Ball S01E04 HDTV MULTISUB" {
			hdtv = &evaluated[i]
			break
		}
	}
	assert.NotNil(t, hdtv)
	assert.False(t, hdtv.PassedFilter)
	assert.True(t, hdtv.EpisodeMatched)
	assert.Equal(t, "rejected: hdtv", hdtv.RejectionReason)

	// S02E04 doesn't match target season 1 episode 4
	var wrongEp *EvaluatedResult
	for i := range evaluated {
		if evaluated[i].Filename == "Dragon Ball S02E04 MULTISUB" {
			wrongEp = &evaluated[i]
			break
		}
	}
	assert.NotNil(t, wrongEp)
	assert.False(t, wrongEp.PassedFilter)
	assert.False(t, wrongEp.EpisodeMatched)
	assert.Equal(t, "Episode mismatch", wrongEp.RejectionReason)
}

