package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectShowType(t *testing.T) {
	tests := []struct {
		name     string
		genres   []string
		rawType  []string
		expected string
	}{
		{
			name:     "empty genres and type defaults to standard",
			genres:   nil,
			rawType:  nil,
			expected: ShowTypeStandard,
		},
		{
			name:     "standard drama show",
			genres:   []string{"Drama", "Crime", "Mystery"},
			rawType:  []string{"Scripted"},
			expected: ShowTypeStandard,
		},
		{
			name:     "animation show but not anime",
			genres:   []string{"Comedy", "Family"},
			rawType:  []string{"Animation"},
			expected: ShowTypeStandard,
		},
		{
			name:     "exact Anime genre",
			genres:   []string{"Action", "Anime", "Fantasy"},
			rawType:  []string{"Animation"},
			expected: ShowTypeAnime,
		},
		{
			name:     "lowercase anime genre",
			genres:   []string{"anime"},
			rawType:  nil,
			expected: ShowTypeAnime,
		},
		{
			name:     "genre with whitespace and mixed case",
			genres:   []string{"  AnImE  "},
			rawType:  nil,
			expected: ShowTypeAnime,
		},
		{
			name:     "comma-separated genre string containing anime",
			genres:   []string{"Action,Anime,Adventure"},
			rawType:  nil,
			expected: ShowTypeAnime,
		},
		{
			name:     "anime rawType",
			genres:   []string{"Action"},
			rawType:  []string{"Anime"},
			expected: ShowTypeAnime,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectShowType(tt.genres, tt.rawType...)
			assert.Equal(t, tt.expected, result)
		})
	}
}
