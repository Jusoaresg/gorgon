package service

import (
	"testing"

	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
	"github.com/jusoaresg/gorgon/internal/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchShowTitle_CanonicalAndAliases(t *testing.T) {
	ctx := filter.Context{
		Show:     "frieren beyond journeys end",
		Aliases:  []string{"sousou no frieren", "frieren"},
		Season:   1,
		Episode:  5,
		ShowType: "anime",
	}

	// Matches canonical name and aliases in standard scene and anime formats
	assert.True(t, MatchShowTitle("Frieren.Beyond.Journeys.End.S01E05.1080p.WEB-DL", ctx))
	assert.True(t, MatchShowTitle("[SubsPlease] Sousou no Frieren - 05 (1080p) [ABCD1234].mkv", ctx))
	assert.True(t, MatchShowTitle("Frieren.2023.S01E05.1080p.WEB-DL", ctx))

	// Rejects other shows with the same episode number
	assert.False(t, MatchShowTitle("[SubsPlease] One Piece - 05 (1080p).mkv", ctx))
	assert.False(t, MatchShowTitle("Breaking.Bad.S01E05.1080p.WEB-DL", ctx))
}

func TestMatchShowTitle_WordBoundaryProtection(t *testing.T) {
	ctx := filter.Context{
		Show:     "dark",
		Aliases:  []string{"er"},
		Season:   1,
		Episode:  2,
		ShowType: "standard",
	}

	// Exact token matches
	assert.True(t, MatchShowTitle("Dark.S01E02.1080p.NF.WEB-DL", ctx))
	assert.True(t, MatchShowTitle("ER.S01E02.720p.WEB-DL", ctx))

	// Substring false positives must be rejected
	assert.False(t, MatchShowTitle("Darker.Than.Black.S01E02.1080p", ctx))
	assert.False(t, MatchShowTitle("Mother.S01E02.1080p.WEB-DL", ctx))
}

func TestFilterAndScoreResponses_MixedRssFeed(t *testing.T) {
	ctx := filter.Context{
		Show:     "dandadan",
		Aliases:  []string{"dan da dan"},
		Season:   1,
		Episode:  3,
		ShowType: "anime",
	}

	profile := &filter.Profile{
		Name: "Anime 1080p",
		Preferred: []filter.PreferredPattern{
			{Pattern: "subsplease", Score: 30},
		},
	}

	rssFeed := []schema.SearchResponse{
		{Title: "[SubsPlease] Bleach Sennen Kessen-hen - 03 (1080p).mkv", Seeders: 100},
		{Title: "[Erai-raws] Dan Da Dan - 03 [720p].mkv", Seeders: 50},
		{Title: "[SubsPlease] Dandadan - 03 (1080p) [1234ABCD].mkv", Seeders: 80},
		{Title: "[SubsPlease] Dandadan - 04 (1080p) [5678EFGH].mkv", Seeders: 90},
	}

	filtered := FilterAndScoreResponses(rssFeed, profile, ctx)
	require.Len(t, filtered, 2)

	// SubsPlease 1080p should rank first over Erai-raws 720p
	assert.Equal(t, "[SubsPlease] Dandadan - 03 (1080p) [1234ABCD].mkv", filtered[0].Title)
	assert.Equal(t, "[Erai-raws] Dan Da Dan - 03 [720p].mkv", filtered[1].Title)
}
