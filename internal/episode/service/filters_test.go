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
}

func TestCalculateScore_IntrinsicQualityOnly(t *testing.T) {
	ctx := filterTestContext()
	profile := filterTestProfile() // web: 30, 1080p: 20

	// Release with 0 seeders
	r1 := schema.SearchResponse{
		Filename: "Dragon Ball S01E04 1080p MULTISUB WEB",
		Seeders:  0,
		Leechers: 10,
	}
	// Same release with 100 seeders
	r2 := schema.SearchResponse{
		Filename: "Dragon Ball S01E04 1080p MULTISUB WEB",
		Seeders:  100,
		Leechers: 0,
	}

	score1 := CalculateScore(r1, profile, ctx)
	score2 := CalculateScore(r2, profile, ctx)

	// Intrinsic score must be identical (50 preferred + 20 resolution = 70) regardless of swarm health
	assert.Equal(t, 70, score1)
	assert.Equal(t, 70, score2)
}

func TestShouldUpgrade(t *testing.T) {
	profileWithCutoff := &filter.Profile{UpgradeUntilScore: 60}
	profileNoCutoff := &filter.Profile{UpgradeUntilScore: 0}

	tests := []struct {
		name           string
		tracking       string
		profile        *filter.Profile
		currentScore   int
		candidateScore int
		expected       bool
	}{
		{
			name:           "Valid upgrade below cutoff",
			tracking:       "downloaded",
			profile:        profileWithCutoff,
			currentScore:   20,
			candidateScore: 50,
			expected:       true,
		},
		{
			name:           "Valid upgrade for snatched episode",
			tracking:       "snatched",
			profile:        profileWithCutoff,
			currentScore:   20,
			candidateScore: 50,
			expected:       true,
		},
		{
			name:           "Skip upgrade when cutoff is disabled (0)",
			tracking:       "downloaded",
			profile:        profileNoCutoff,
			currentScore:   20,
			candidateScore: 50,
			expected:       false,
		},
		{
			name:           "Skip upgrade when profile is nil",
			tracking:       "downloaded",
			profile:        nil,
			currentScore:   20,
			candidateScore: 50,
			expected:       false,
		},
		{
			name:           "Skip upgrade when current score reached cutoff",
			tracking:       "downloaded",
			profile:        profileWithCutoff,
			currentScore:   60,
			candidateScore: 70,
			expected:       false,
		},
		{
			name:           "Skip upgrade when candidate score is lower",
			tracking:       "downloaded",
			profile:        profileWithCutoff,
			currentScore:   50,
			candidateScore: 30,
			expected:       false,
		},
		{
			name:           "Skip upgrade when candidate score is equal",
			tracking:       "downloaded",
			profile:        profileWithCutoff,
			currentScore:   50,
			candidateScore: 50,
			expected:       false,
		},
		{
			name:           "Skip upgrade for un-downloaded tracking (wanted)",
			tracking:       "wanted",
			profile:        profileWithCutoff,
			currentScore:   0,
			candidateScore: 50,
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldUpgrade(tt.tracking, tt.profile, tt.currentScore, tt.candidateScore)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestReleaseName_FallbackToTitle(t *testing.T) {
	withFilename := schema.SearchResponse{Filename: "custom.mkv", Title: "fallback.mkv"}
	assert.Equal(t, "custom.mkv", releaseName(withFilename))

	withoutFilename := schema.SearchResponse{Filename: "", Title: "title.mkv"}
	assert.Equal(t, "title.mkv", releaseName(withoutFilename))
}

func TestGetQualityScore_4KSupport(t *testing.T) {
	r4k := schema.SearchResponse{Title: "Show.S01E01.2160p.UHD.WEB-DL.mkv"}
	assert.Equal(t, 30, getQualityScore(r4k))

	r1080 := schema.SearchResponse{Title: "Show.S01E01.1080p.WEB-DL.mkv"}
	assert.Equal(t, 20, getQualityScore(r1080))

	r720 := schema.SearchResponse{Title: "Show.S01E01.720p.HDTV.mkv"}
	assert.Equal(t, 10, getQualityScore(r720))

	r480 := schema.SearchResponse{Title: "Show.S01E01.480p.mkv"}
	assert.Equal(t, 0, getQualityScore(r480))
}

func TestEvaluateResponse_UsesTitleWhenFilenameEmpty(t *testing.T) {
	ctx := filterTestContext()
	profile := filterTestProfile() // web: 30, 1080p: 20

	// Release with empty Filename, Title only (standard Prowlarr format)
	r := schema.SearchResponse{
		Title:   "Dragon Ball S01E04 1080p MULTISUB WEB",
		Seeders: 25,
	}

	result := EvaluateResponse(r, profile, ctx)
	assert.True(t, result.PassedFilter)
	assert.True(t, result.EpisodeMatched)
	assert.Equal(t, "1080P", result.Metadata.Resolution)
	// Score must match canonical CalculateScore (30 web + 20 1080p + 20 resolution = 70)
	assert.Equal(t, 70, result.Score)
}
