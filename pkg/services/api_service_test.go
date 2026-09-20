package services

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jusoaresg/gorgon/pkg/schemas/dtos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestNewAPIService_Options(t *testing.T) {
	customClient := &http.Client{Timeout: 5 * time.Second}
	headers := map[string]string{"X-Custom": "GorgonHeader"}

	svc := NewAPIService("http://localhost:8080", nil,
		WithTimeout(10*time.Second),
		WithHTTPClient(customClient),
		WithDefaultHeaders(headers),
	)

	assert.Equal(t, customClient, svc.Client)
	assert.Equal(t, "GorgonHeader", svc.DefaultHeaders["X-Custom"])
}

func TestAPIService_BuildURL(t *testing.T) {
	svc := NewAPIService("http://api.tvmaze.com/", nil)

	assert.Equal(t, "http://api.tvmaze.com/search/shows?q=test", svc.buildURL("/search/shows?q=test"))
	assert.Equal(t, "http://api.tvmaze.com/search/shows?q=test", svc.buildURL("search/shows?q=test"))
	assert.Equal(t, "http://api.tvmaze.com?q=test", svc.buildURL("?q=test"))
	assert.Equal(t, "http://api.tvmaze.com", svc.buildURL(""))

	svcNoSlash := NewAPIService("http://localhost:9696", nil)
	assert.Equal(t, "http://localhost:9696/api/v1/search", svcNoSlash.buildURL("/api/v1/search"))
	assert.Equal(t, "http://localhost:9696/api/v1/search", svcNoSlash.buildURL("api/v1/search"))
}

type sampleResponse struct {
	Status  string `json:"status"`
	Count   int    `json:"count"`
	Message string `json:"message,omitempty"`
}

type sampleRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

func TestAPIService_Get(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)

		switch r.URL.Path {
		case "/api/json":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(sampleResponse{Status: "ok", Count: 42})
		case "/api/text":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("raw text response"))
		case "/api/bytes":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte{0x01, 0x02, 0x03, 0x04})
		case "/api/empty":
			w.WriteHeader(http.StatusOK)
		case "/api/error":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error": "show not found"}`))
		case "/api/bad-json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`invalid json {{{`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	svc := NewAPIService(server.URL, logger)

	t.Run("successful JSON unmarshaling", func(t *testing.T) {
		var resp sampleResponse
		err := svc.Get("/api/json", &resp)
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Status)
		assert.Equal(t, 42, resp.Count)
	})

	t.Run("successful *string decoding", func(t *testing.T) {
		var text string
		err := svc.Get("/api/text", &text)
		require.NoError(t, err)
		assert.Equal(t, "raw text response", text)
	})

	t.Run("successful *[]byte decoding", func(t *testing.T) {
		var b []byte
		err := svc.Get("/api/bytes", &b)
		require.NoError(t, err)
		assert.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, b)
	})

	t.Run("successful nil target", func(t *testing.T) {
		err := svc.Get("/api/empty", nil)
		require.NoError(t, err)
	})

	t.Run("HTTP 404 returns structured APIError", func(t *testing.T) {
		var resp sampleResponse
		err := svc.Get("/api/error", &resp)
		require.Error(t, err)

		var apiErr *APIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
		assert.Equal(t, http.MethodGet, apiErr.Method)
		assert.Contains(t, apiErr.Body, "show not found")
	})

	t.Run("invalid JSON response returns error", func(t *testing.T) {
		var resp sampleResponse
		err := svc.Get("/api/bad-json", &resp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error while decoding response body")
	})
}

func TestAPIService_GetWithHeaders(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token-123", r.Header.Get("Authorization"))
		assert.Equal(t, "GorgonClient", r.Header.Get("X-Client-App"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(sampleResponse{Status: "authenticated"})
	}))
	defer server.Close()

	svc := NewAPIService(server.URL, logger)
	headers := map[string]string{
		"Authorization": "Bearer test-token-123",
		"X-Client-App":  "GorgonClient",
	}

	var resp sampleResponse
	err := svc.GetWithHeaders("/api/auth", &resp, headers)
	require.NoError(t, err)
	assert.Equal(t, "authenticated", resp.Status)
}

func TestAPIService_GetWithHeadersRaw(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "qbittorrent-version-test", r.Header.Get("X-Qbit-Header"))
		w.Header().Set("X-Custom-Header", "value123")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("v4.5.2"))
	}))
	defer server.Close()

	svc := NewAPIService(server.URL, logger)
	resp, err := svc.GetWithHeadersRaw("/api/version", map[string]string{"X-Qbit-Header": "qbittorrent-version-test"})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "value123", resp.Header.Get("X-Custom-Header"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "v4.5.2", string(body))
}

func TestAPIService_Post(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)

		switch r.URL.Path {
		case "/api/json":
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			var req sampleRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			assert.NoError(t, err)
			assert.Equal(t, "anime", req.Query)
			assert.Equal(t, 10, req.Limit)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(sampleResponse{Status: "created", Count: 1})

		case "/api/form":
			bodyBytes, _ := io.ReadAll(r.Body)
			assert.Equal(t, "key=value&foo=bar", string(bodyBytes))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))

		case "/api/bytes":
			bodyBytes, _ := io.ReadAll(r.Body)
			assert.Equal(t, []byte{0xDE, 0xAD, 0xBE, 0xEF}, bodyBytes)
			w.WriteHeader(http.StatusCreated)

		case "/api/error":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": "invalid payload"}`))
		}
	}))
	defer server.Close()

	svc := NewAPIService(server.URL, logger)

	t.Run("POST struct as JSON", func(t *testing.T) {
		var resp sampleResponse
		req := sampleRequest{Query: "anime", Limit: 10}
		httpResp, err := svc.Post("/api/json", req, &resp)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, httpResp.StatusCode)
		assert.Equal(t, "created", resp.Status)
		assert.Equal(t, 1, resp.Count)
	})

	t.Run("POST string payload (form)", func(t *testing.T) {
		var resp string
		httpResp, err := svc.Post("/api/form", "key=value&foo=bar", &resp)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, httpResp.StatusCode)
		assert.Equal(t, "ok", resp)
	})

	t.Run("POST []byte payload", func(t *testing.T) {
		httpResp, err := svc.Post("/api/bytes", []byte{0xDE, 0xAD, 0xBE, 0xEF}, nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, httpResp.StatusCode)
	})

	t.Run("POST io.Reader payload", func(t *testing.T) {
		var resp string
		httpResp, err := svc.Post("/api/form", strings.NewReader("key=value&foo=bar"), &resp)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, httpResp.StatusCode)
		assert.Equal(t, "ok", resp)
	})

	t.Run("POST with invalid JSON response", func(t *testing.T) {
		var resp sampleResponse
		_, err := svc.Post("/api/form", "key=value&foo=bar", &resp)
		require.Error(t, err)
	})

	t.Run("POST marshaling error", func(t *testing.T) {
		_, err := svc.Post("/api/json", make(chan int), nil)
		require.Error(t, err)
	})
}

func TestAPIService_PostRaw(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		assert.Equal(t, "custom-header-val", r.Header.Get("X-Custom-Test"))

		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, "hashes=abc1234", string(body))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("deleted"))
	}))
	defer server.Close()

	svc := NewAPIService(server.URL, logger)
	resp, err := svc.PostRaw(
		"/api/torrents/delete",
		"application/x-www-form-urlencoded",
		strings.NewReader("hashes=abc1234"),
		map[string]string{"X-Custom-Test": "custom-header-val"},
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "deleted", string(body))
}

func TestAPIService_ContextCancellation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	svc := NewAPIService(server.URL, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	var resp sampleResponse
	err := svc.GetContext(ctx, "/slow", &resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), context.DeadlineExceeded.Error())
}

func TestAPIService_WithTimeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	svc := NewAPIService(server.URL, logger, WithTimeout(30*time.Millisecond))

	var resp sampleResponse
	err := svc.Get("/slow", &resp)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") || strings.Contains(err.Error(), "Client.Timeout"))
}

func TestAPIError_Formatting(t *testing.T) {
	apiErr := &APIError{
		StatusCode: 502,
		Method:     "GET",
		URL:        "http://api.tvmaze.com/shows/123",
		Body:       "Bad Gateway",
	}

	expected := "API request GET http://api.tvmaze.com/shows/123 failed with status 502: Bad Gateway"
	assert.Equal(t, expected, apiErr.Error())
}

func TestApplyTrackingToEpisodes(t *testing.T) {
	pastTime := time.Now().Add(-48 * time.Hour).Format(time.RFC3339)
	futureTime := time.Now().Add(48 * time.Hour).Format(time.RFC3339)

	t.Run("tracking all marks all episodes as wanted", func(t *testing.T) {
		episodes := []dtos.EpisodeDto{
			{Name: "Ep 1", AirStamp: pastTime},
			{Name: "Ep 2", AirStamp: futureTime},
		}
		ApplyTrackingToEpisodes(&episodes, "all")
		assert.Equal(t, "wanted", episodes[0].Tracking)
		assert.Equal(t, "wanted", episodes[1].Tracking)
	})

	t.Run("tracking none marks all episodes as skipped", func(t *testing.T) {
		episodes := []dtos.EpisodeDto{
			{Name: "Ep 1", AirStamp: pastTime},
			{Name: "Ep 2", AirStamp: futureTime},
		}
		ApplyTrackingToEpisodes(&episodes, "none")
		assert.Equal(t, "skipped", episodes[0].Tracking)
		assert.Equal(t, "skipped", episodes[1].Tracking)
	})

	t.Run("tracking future marks past as skipped and future as wanted", func(t *testing.T) {
		episodes := []dtos.EpisodeDto{
			{Name: "Ep 1", AirStamp: pastTime},
			{Name: "Ep 2", AirStamp: futureTime},
		}
		ApplyTrackingToEpisodes(&episodes, "future")
		assert.Equal(t, "skipped", episodes[0].Tracking)
		assert.Equal(t, "wanted", episodes[1].Tracking)
	})

	t.Run("invalid airstamp falls back to wanted", func(t *testing.T) {
		episodes := []dtos.EpisodeDto{
			{Name: "Ep 1", AirStamp: "invalid-date"},
		}
		ApplyTrackingToEpisodes(&episodes, "future")
		assert.Equal(t, "wanted", episodes[0].Tracking)
	})
}

type mockConnectable struct {
	name string
	err  error
}

func (m *mockConnectable) Name() string {
	return m.name
}

func (m *mockConnectable) CheckConnection() error {
	return m.err
}

func TestCheckAllConnections(t *testing.T) {
	t.Run("all connections healthy", func(t *testing.T) {
		svc1 := &mockConnectable{name: "Prowlarr", err: nil}
		svc2 := &mockConnectable{name: "qBittorrent", err: nil}

		errs := CheckAllConnections(svc1, svc2)
		assert.Empty(t, errs)
	})

	t.Run("some connection failing", func(t *testing.T) {
		svc1 := &mockConnectable{name: "Prowlarr", err: nil}
		svc2 := &mockConnectable{name: "qBittorrent", err: assert.AnError}

		errs := CheckAllConnections(svc1, svc2)
		require.Len(t, errs, 1)
		assert.Equal(t, "qBittorrent", errs[0].ServiceName)
		assert.Equal(t, assert.AnError.Error(), errs[0].Error)
	})
}
