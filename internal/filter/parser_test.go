package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseReleaseMetadata(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		expected ReleaseMetadata
	}{
		{
			name:     "Anime Erai-raws Multi-Sub 1080p",
			filename: "[Erai-raws] Jujutsu Kaisen 2nd Season - 05 [1080p][Multiple Subtitle].mkv",
			expected: ReleaseMetadata{
				ReleaseGroup: "Erai-raws",
				Resolution:   "1080P",
				Languages:    []string{"Multi-Sub"},
			},
		},
		{
			name:     "Anime SubsPlease 1080p with CRC",
			filename: "[SubsPlease] Chainsaw Man - 08 (1080p) [26EF07AE].mkv",
			expected: ReleaseMetadata{
				ReleaseGroup: "SubsPlease",
				Resolution:   "1080P",
			},
		},
		{
			name:     "Standard TV Scene Release with FLUX and WEB-DL",
			filename: "Breaking.Bad.S01E05.1080p.WEB-DL.x264-FLUX.mkv",
			expected: ReleaseMetadata{
				ReleaseGroup: "FLUX",
				Resolution:   "1080P",
				Source:       "WEB-DL",
				Codec:        "X264",
			},
		},
		{
			name:     "PT-BR Dual Audio release",
			filename: "One.Piece.E1080.1080p.Dual.Audio.PT-BR.mkv",
			expected: ReleaseMetadata{
				Resolution: "1080P",
				Audio:      "DUAL AUDIO",
				Languages:  []string{"PT-BR", "Dual Audio"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseReleaseMetadata(tc.filename)
			if tc.expected.ReleaseGroup != "" {
				assert.Equal(t, tc.expected.ReleaseGroup, got.ReleaseGroup)
			}
			if tc.expected.Resolution != "" {
				assert.Equal(t, tc.expected.Resolution, got.Resolution)
			}
			if tc.expected.Source != "" {
				assert.Equal(t, tc.expected.Source, got.Source)
			}
			if tc.expected.Codec != "" {
				assert.Equal(t, tc.expected.Codec, got.Codec)
			}
			for _, lang := range tc.expected.Languages {
				assert.Contains(t, got.Languages, lang)
			}
		})
	}
}
