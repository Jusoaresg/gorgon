package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// APIService provides a robust, reusable HTTP client for interacting with internal and external APIs.
type APIService struct {
	Url            string
	Client         *http.Client
	Logger         *slog.Logger
	DefaultHeaders map[string]string
}

// APIError represents an HTTP error with response status code and body details.
type APIError struct {
	StatusCode int
	Method     string
	URL        string
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API request %s %s failed with status %d: %s", e.Method, e.URL, e.StatusCode, e.Body)
}

// Option configures an APIService instance.
type Option func(*APIService)

// WithTimeout configures the HTTP client timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(a *APIService) {
		if a.Client != nil {
			a.Client.Timeout = timeout
		}
	}
}

// WithHTTPClient overrides the underlying http.Client.
func WithHTTPClient(client *http.Client) Option {
	return func(a *APIService) {
		if client != nil {
			a.Client = client
		}
	}
}

// WithDefaultHeaders sets default headers to be sent with every request.
func WithDefaultHeaders(headers map[string]string) Option {
	return func(a *APIService) {
		for k, v := range headers {
			a.DefaultHeaders[k] = v
		}
	}
}

// NewAPIService creates a new APIService with normalized base URL and structured logging.
func NewAPIService(url string, logger *slog.Logger, opts ...Option) *APIService {
	url = strings.TrimSpace(url)
	if url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}

	if logger == nil {
		logger = slog.Default()
	}

	svc := &APIService{
		Url: url,
		Client: &http.Client{
			Timeout: 60 * time.Second,
		},
		Logger:         logger.WithGroup("apiService"),
		DefaultHeaders: make(map[string]string),
	}

	for _, opt := range opts {
		opt(svc)
	}

	return svc
}

func (a *APIService) buildURL(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	base := strings.TrimRight(strings.TrimSpace(a.Url), "/")
	if endpoint == "" {
		return base
	}
	if !strings.HasPrefix(endpoint, "/") && !strings.HasPrefix(endpoint, "?") {
		return base + "/" + endpoint
	}
	if strings.HasPrefix(endpoint, "?") {
		return base + endpoint
	}
	return base + endpoint
}

func decodeResponseBody(body []byte, target any) error {
	if target == nil {
		return nil
	}
	if len(body) == 0 {
		return nil
	}
	switch v := target.(type) {
	case *string:
		*v = string(body)
		return nil
	case *[]byte:
		*v = append([]byte(nil), body...)
		return nil
	default:
		if err := json.Unmarshal(body, target); err != nil {
			return fmt.Errorf("error while decoding response body: %w. Raw body: %s", err, string(body))
		}
		return nil
	}
}

func (a *APIService) doRequest(ctx context.Context, method, endpoint string, reqBody io.Reader, reqBytes []byte, headers map[string]string) (*http.Response, []byte, error) {
	fullURL := a.buildURL(endpoint)

	if ctx == nil {
		ctx = context.Background()
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		a.Logger.Error("Failed to create request",
			slog.String("method", method),
			slog.String("url", fullURL),
			slog.String("error", err.Error()),
		)
		return nil, nil, fmt.Errorf("error creating %s request: %w", method, err)
	}

	for k, v := range a.DefaultHeaders {
		req.Header.Set(k, v)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := a.Client.Do(req)
	if err != nil {
		a.Logger.Error(method+" request failed",
			slog.String("url", fullURL),
			slog.String("error", err.Error()),
		)
		return nil, nil, fmt.Errorf("error while making %s request: %w", method, err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		a.Logger.Error("Error reading response body",
			slog.String("url", fullURL),
			slog.Int("status", resp.StatusCode),
			slog.String("error", err.Error()),
		)
		return resp, nil, fmt.Errorf("error reading response body: %w", err)
	}

	logAttrs := []any{
		slog.String("url", fullURL),
		slog.Int("status", resp.StatusCode),
	}
	if len(reqBytes) > 0 {
		logAttrs = append(logAttrs, slog.String("request_body", string(reqBytes)))
	}
	logAttrs = append(logAttrs, slog.String("response_body", string(bodyBytes)))
	a.Logger.Debug(method+" response", logAttrs...)

	return resp, bodyBytes, nil
}

// Get performs a GET request with background context and unmarshals the response.
func (a *APIService) Get(endpoint string, response any) error {
	return a.GetContext(context.Background(), endpoint, response)
}

// GetContext performs a context-aware GET request and unmarshals the response.
func (a *APIService) GetContext(ctx context.Context, endpoint string, response any, headersInfo ...map[string]string) error {
	var headers map[string]string
	if len(headersInfo) > 0 && headersInfo[0] != nil {
		headers = headersInfo[0]
	}

	resp, bodyBytes, err := a.doRequest(ctx, http.MethodGet, endpoint, nil, nil, headers)
	if err != nil {
		return err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return &APIError{
			StatusCode: resp.StatusCode,
			Method:     http.MethodGet,
			URL:        a.buildURL(endpoint),
			Body:       string(bodyBytes),
		}
	}

	if err := decodeResponseBody(bodyBytes, response); err != nil {
		a.Logger.Error("Error decoding GET response body",
			slog.String("url", a.buildURL(endpoint)),
			slog.Int("status", resp.StatusCode),
			slog.String("error", err.Error()),
		)
		return err
	}

	a.Logger.Debug("GET request successful",
		slog.String("url", a.buildURL(endpoint)),
		slog.Int("status", resp.StatusCode),
	)

	return nil
}

// GetWithHeaders performs a GET request with explicit headers.
func (a *APIService) GetWithHeaders(endpoint string, response any, headers map[string]string) error {
	return a.GetContext(context.Background(), endpoint, response, headers)
}

// GetWithHeadersRaw performs a GET request and returns the raw response. The caller owns the response body.
func (a *APIService) GetWithHeadersRaw(endpoint string, headers map[string]string) (*http.Response, error) {
	url := a.buildURL(endpoint)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("error while creating GET request: %w", err)
	}

	for k, v := range a.DefaultHeaders {
		req.Header.Set(k, v)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error while making GET request: %w", err)
	}

	return resp, nil
}

// PostRaw performs a POST request and returns the raw response without
// consuming or closing its body. The caller owns the response body.
func (a *APIService) PostRaw(endpoint, contentType string, body io.Reader, headers map[string]string) (*http.Response, error) {
	url := a.buildURL(endpoint)

	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("error while creating POST request: %w", err)
	}

	for k, v := range a.DefaultHeaders {
		req.Header.Set(k, v)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := a.Client.Do(req)
	if err != nil {
		a.Logger.Error("POST request failed",
			slog.String("url", url),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("error while making POST request: %w", err)
	}

	return resp, nil
}

// Post performs a POST request with background context and unmarshals the response if status is 2xx.
func (a *APIService) Post(endpoint string, requestData any, response any, headersInfo ...map[string]string) (*http.Response, error) {
	return a.PostContext(context.Background(), endpoint, requestData, response, headersInfo...)
}

// PostContext performs a context-aware POST request and unmarshals the response if status is 2xx.
func (a *APIService) PostContext(ctx context.Context, endpoint string, requestData any, response any, headersInfo ...map[string]string) (*http.Response, error) {
	var reqBody io.Reader
	var reqBytes []byte
	isJSON := false

	if requestData != nil {
		switch v := requestData.(type) {
		case string:
			reqBody = strings.NewReader(v)
			reqBytes = []byte(v)
		case []byte:
			reqBody = bytes.NewReader(v)
			reqBytes = v
		case io.Reader:
			reqBody = v
		default:
			jsonData, err := json.Marshal(requestData)
			if err != nil {
				a.Logger.Error("Error marshaling POST request data",
					slog.String("url", a.buildURL(endpoint)),
					slog.String("error", err.Error()),
				)
				return nil, fmt.Errorf("error while marshaling request data: %w", err)
			}
			reqBody = bytes.NewReader(jsonData)
			reqBytes = jsonData
			isJSON = true
		}
	}

	headers := make(map[string]string)
	if isJSON {
		headers["Content-Type"] = "application/json"
	}

	if len(headersInfo) > 0 {
		for _, h := range headersInfo {
			for key, value := range h {
				headers[key] = value
			}
		}
	}

	resp, bodyBytes, err := a.doRequest(ctx, http.MethodPost, endpoint, reqBody, reqBytes, headers)
	if err != nil {
		return resp, err
	}

	if response != nil && resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		if err := decodeResponseBody(bodyBytes, response); err != nil {
			a.Logger.Error("Error decoding POST response body",
				slog.String("url", a.buildURL(endpoint)),
				slog.Int("status", resp.StatusCode),
				slog.String("error", err.Error()),
			)
			return resp, err
		}
	}

	a.Logger.Info("POST request successful",
		slog.String("url", a.buildURL(endpoint)),
		slog.Int("status", resp.StatusCode),
	)

	return resp, nil
}
