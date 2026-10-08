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
		{
			name:        "Standard standalone E05 wanted S1E05",
			release:     "Breaking.Bad.E05.1080p.WEB-DL.x264-GRP.mkv",
			season:      1,
			episode:     5,
			showType:    "standard",
			expectMatch: true,
		},
		{
			name:        "Standard standalone lowercase e05 wanted S1E05",
			release:     "Breaking.Bad.e05.1080p.WEB-DL.x264-GRP.mkv",
			season:      1,
			episode:     5,
			showType:    "standard",
			expectMatch: true,
		},
		{
			name:        "Standard Episode word format Episode.05 wanted S1E05",
			release:     "Show.Name.Episode.05.720p.HDTV.x264-GRP.mkv",
			season:      1,
			episode:     5,
			showType:    "standard",
			expectMatch: true,
		},
		{
			name:        "Standard Ep abbreviation format Ep.05 wanted S1E05",
			release:     "Show.Name.Ep.05.720p.HDTV.x264-GRP.mkv",
			season:      1,
			episode:     5,
			showType:    "standard",
			expectMatch: true,
		},
		{
			name:        "Standard Part format Part.01 wanted S1E01",
			release:     "Chernobyl.Part.01.1080p.WEB-DL.x264-GRP.mkv",
			season:      1,
			episode:     1,
			showType:    "standard",
			expectMatch: true,
		},
		{
			name:        "Standard standalone multi-episode E01-E03 wanted S1E02",
			release:     "Show.Name.E01-E03.1080p.mkv",
			season:      1,
			episode:     2,
			showType:    "standard",
			expectMatch: true,
		},
		{
			name:        "Standard standalone multi-episode E01-E03 wanted S1E04 (out of range)",
			release:     "Show.Name.E01-E03.1080p.mkv",
			season:      1,
			episode:     4,
			showType:    "standard",
			expectMatch: false,
		},
		{
			name:        "Standard dot-separated S01.E05 wanted S1E05",
			release:     "Breaking.Bad.S01.E05.1080p.WEB-DL.x264-GRP.mkv",
			season:      1,
			episode:     5,
			showType:    "standard",
			expectMatch: true,
		},
		{
			name:        "Standard separated Season.2.Episode.05 wanted S2E05",
			release:     "Show.Name.Season.2.Episode.05.1080p.mkv",
			season:      2,
			episode:     5,
			showType:    "standard",
			expectMatch: true,
		},
		{
			name:        "Standard separated Season.2.Episode.05 wanted S1E05 (season mismatch)",
			release:     "Show.Name.Season.2.Episode.05.1080p.mkv",
			season:      1,
			episode:     5,
			showType:    "standard",
			expectMatch: false,
		},
		// --- REGRESSION TESTS FOR SEASON-LESS RELEASES (MUST REJECT S2+) ---
		{
			name:        "Regression: Standard E05 without season wanted S2E05 (must reject!)",
			release:     "Breaking.Bad.E05.1080p.WEB-DL.x264-GRP.mkv",
			season:      2,
			episode:     5,
			showType:    "standard",
			expectMatch: false,
		},
		{
			name:        "Regression: Standard Episode.05 without season wanted S2E05 (must reject!)",
			release:     "Show.Name.Episode.05.720p.HDTV.x264-GRP.mkv",
			season:      2,
			episode:     5,
			showType:    "standard",
			expectMatch: false,
		},
		{
			name:        "Regression: Standard E01-E03 without season wanted S2E02 (must reject!)",
			release:     "Show.Name.E01-E03.1080p.mkv",
			season:      2,
			episode:     2,
			showType:    "standard",
			expectMatch: false,
		},
		{
			name:        "Regression: Standard Part.01 without season wanted S2E01 (must reject!)",
			release:     "Chernobyl.Part.01.1080p.WEB-DL.x264-GRP.mkv",
			season:      2,
			episode:     1,
			showType:    "standard",
			expectMatch: false,
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
		// --- REGRESSION TESTS FOR ANIME SEASON-LESS RELEASES (MUST REJECT S2+) ---
		{
			name:        "Regression: Erai-raws episode 05 without season wanted S2E05 (must reject!)",
			release:     "[Erai-raws] Jujutsu Kaisen - 05 [1080p][Multiple Subtitle].mkv",
			season:      2,
			episode:     5,
			expectMatch: false,
		},
		{
			name:        "Regression: SubsPlease episode 08 without season wanted S2E08 (must reject!)",
			release:     "[SubsPlease] Chainsaw Man (2022) - 08 (1080p) [26EF07AE].mkv",
			season:      2,
			episode:     8,
			expectMatch: false,
		},
		{
			name:        "Regression: Word format Episode 12 without season wanted S2E12 (must reject!)",
			release:     "[Group] Anime Name Episode 12 [720p].mkv",
			season:      2,
			episode:     12,
			expectMatch: false,
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
