package services

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewAPIService_SchemeNormalization(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain hostname prepends http://",
			input:    "localhost:8080",
			expected: "http://localhost:8080",
		},
		{
			name:     "ip address prepends http://",
			input:    "192.168.1.100:9696",
			expected: "http://192.168.1.100:9696",
		},
		{
			name:     "docker container name prepends http://",
			input:    "gorgon-qbittorrent:9191",
			expected: "http://gorgon-qbittorrent:9191",
		},
		{
			name:     "existing http:// scheme preserved",
			input:    "http://localhost:8080",
			expected: "http://localhost:8080",
		},
		{
			name:     "existing https:// scheme preserved",
			input:    "https://qbittorrent.example.com",
			expected: "https://qbittorrent.example.com",
		},
		{
			name:     "leading whitespace trimmed",
			input:    "  localhost:9191  ",
			expected: "http://localhost:9191",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewAPIService(tt.input, logger)
			assert.Equal(t, tt.expected, svc.Url)
		})
	}
}
