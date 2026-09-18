package filter

import (
	"testing"
)

func TestMatchEpisode_Standard(t *testing.T) {
	tests := []struct {
		name        string
		release     string
		season      int
		episode     int
		showType    string
		expectMatch bool
	}{
		{
			name:        "Standard exact match S01E12",
			release:     "Breaking.Bad.S01E12.1080p.WEB-DL.x264-GRP.mkv",
			season:      1,
			episode:     12,
			showType:    "standard",
			expectMatch: true,
		},
		{
			name:        "Standard wrong episode S01E05",
			release:     "Breaking.Bad.S01E05.1080p.WEB-DL.x264-GRP.mkv",
			season:      1,
			episode:     12,
			showType:    "standard",
			expectMatch: false,
		},
		{
			name:        "Standard wrong season S02E12",
			release:     "Breaking.Bad.S02E12.1080p.WEB-DL.x264-GRP.mkv",
			season:      1,
			episode:     12,
			showType:    "standard",
			expectMatch: false,
		},
		{
			name:        "Standard 1x12 format",
			release:     "Lost.1x12.720p.HDTV.x264-GRP.mkv",
			season:      1,
			episode:     12,
			showType:    "standard",
			expectMatch: true,
		},
		{
			name:        "Standard multi-episode S01E11-E13 match",
			release:     "Show.S01E11-E13.1080p.mkv",
			season:      1,
			episode:     12,
			showType:    "standard",
			expectMatch: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchEpisode(tc.release, tc.season, tc.episode, tc.showType)
			if got != tc.expectMatch {
				t.Errorf("MatchEpisode(%q, S%d, E%d, %s) = %v, want %v", tc.release, tc.season, tc.episode, tc.showType, got, tc.expectMatch)
			}
		})
	}
}

func TestMatchEpisode_Anime(t *testing.T) {
	tests := []struct {
		name        string
		release     string
		season      int
		episode     int
		expectMatch bool
	}{
		{
			name:        "Erai-raws episode 12 wanted 12",
			release:     "[Erai-raws] Jujutsu Kaisen - 12 [1080p][Multiple Subtitle].mkv",
			season:      1,
			episode:     12,
			expectMatch: true,
		},
		{
			name:        "Erai-raws episode 05 wanted 12 (must reject!)",
			release:     "[Erai-raws] Jujutsu Kaisen - 05 [1080p][Multiple Subtitle].mkv",
			season:      1,
			episode:     12,
			expectMatch: false,
		},
		{
			name:        "Erai-raws Season 2 episode 05 wanted S2E05",
			release:     "[Erai-raws] Jujutsu Kaisen 2nd Season - 05 [1080p][Multiple Subtitle].mkv",
			season:      2,
			episode:     5,
			expectMatch: true,
		},
		{
			name:        "Erai-raws Season 2 episode 05 wanted S1E05 (season mismatch)",
			release:     "[Erai-raws] Jujutsu Kaisen 2nd Season - 05 [1080p][Multiple Subtitle].mkv",
			season:      1,
			episode:     5,
			expectMatch: false,
		},
		{
			name:        "SubsPlease with CRC hash and year",
			release:     "[SubsPlease] Chainsaw Man (2022) - 08 (1080p) [26EF07AE].mkv",
			season:      1,
			episode:     8,
			expectMatch: true,
		},
		{
			name:        "SubsPlease S2 style",
			release:     "[SubsPlease] Jujutsu Kaisen S2 - 12 (1080p) [12345678].mkv",
			season:      2,
			episode:     12,
			expectMatch: true,
		},
		{
			name:        "Anime with S01E12 style",
			release:     "[Judas] Jujutsu Kaisen - S01E12 [1080p].mkv",
			season:      1,
			episode:     12,
			expectMatch: true,
		},
		{
			name:        "Episode with v2 suffix",
			release:     "[Erai-raws] One Piece - 1080v2 [1080p].mkv",
			season:      1,
			episode:     1080,
			expectMatch: true,
		},
		{
			name:        "Word format Episode 12",
			release:     "[Group] Anime Name Episode 12 [720p].mkv",
			season:      1,
			episode:     12,
			expectMatch: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchEpisode(tc.release, tc.season, tc.episode, "anime")
			if got != tc.expectMatch {
				t.Errorf("MatchEpisode(%q, S%d, E%d, anime) = %v, want %v", tc.release, tc.season, tc.episode, got, tc.expectMatch)
			}
		})
	}
}
