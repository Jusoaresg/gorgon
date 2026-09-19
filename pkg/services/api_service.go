package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type APIService struct {
	Url    string
	Client *http.Client
	Logger *slog.Logger
}

func NewAPIService(url string, logger *slog.Logger) (a *APIService) {
	url = strings.TrimSpace(url)
	if url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}
	return &APIService{
		Url: url,
		Client: &http.Client{
			Timeout: 60 * time.Second,
		},
		Logger: logger.WithGroup("apiService"),
	}
}

func (a *APIService) Get(endpoint string, response any) error {
	url := fmt.Sprintf("%s%s", a.Url, endpoint)

	resp, err := a.Client.Get(url)
	if err != nil {
		a.Logger.Error("GET request failed",
			slog.String("url", url),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("Error while get request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		a.Logger.Error("Error reading GET response body",
			slog.String("url", url),
			slog.Int("status", resp.StatusCode),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("Error reading response body: %w", err)
	}

	a.Logger.Debug("GET response",
		slog.String("url", url),
		slog.Int("status", resp.StatusCode),
	)

	if err := json.Unmarshal(body, &response); err != nil {
		a.Logger.Error("Error decoding GET response body",
			slog.String("url", url),
			slog.Int("status", resp.StatusCode),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("Error while decoding response body: %w", err)
	}

	a.Logger.Debug("GET request successful",
		slog.String("url", url),
		slog.Int("status", resp.StatusCode),
	)

	return nil
}

func (a *APIService) GetWithHeaders(endpoint string, response any, headers map[string]string) error {
	url := fmt.Sprintf("%s%s", a.Url, endpoint)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("Error while creating GET request: %w", err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := a.Client.Do(req)
	if err != nil {
		return fmt.Errorf("Error while making GET request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Error reading response body: %w", err)
	}

	if len(body) == 0 {
		return fmt.Errorf("Empty response body")
	}

	if err := json.Unmarshal(body, &response); err != nil {
		if strResp, ok := response.(*string); ok {
			*strResp = string(body)
			return nil
		}

		return fmt.Errorf("Error while decoding response body: %w. Raw body: %s", err, string(body))
	}

	return nil
}

func (a *APIService) GetWithHeadersRaw(endpoint string, headers map[string]string) (*http.Response, error) {
	url := fmt.Sprintf("%s%s", a.Url, endpoint)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("Error while creating GET request: %w", err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error while making GET request: %w", err)
	}

	return resp, nil
}

// PostRaw performs a POST request and returns the raw response without
// consuming or closing its body. The caller owns the response body.
func (a *APIService) PostRaw(endpoint, contentType string, body io.Reader, headers map[string]string) (*http.Response, error) {
	url := fmt.Sprintf("%s%s", a.Url, endpoint)

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("Error while creating POST request: %w", err)
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
		return nil, fmt.Errorf("Error while making POST request: %w", err)
	}

	return resp, nil
}

func (a *APIService) Post(endpoint string, requestData any, response any, headersInfo ...map[string]string) (*http.Response, error) {
	url := fmt.Sprintf("%s%s", a.Url, endpoint)

	var reqBody io.Reader
	var requestBodyBytes []byte

	switch v := requestData.(type) {
	case string:
		reqBody = strings.NewReader(v)
	case []byte:
		reqBody = bytes.NewBuffer(v)
	default:
		jsonData, err := json.Marshal(requestData)
		if err != nil {
			a.Logger.Error("Error marshaling POST request data",
				slog.String("url", url),
				slog.String("error", err.Error()),
			)
			return nil, fmt.Errorf("error while marshaling request data: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
		requestBodyBytes = jsonData
	}

	req, err := http.NewRequest("POST", url, reqBody)
	if err != nil {
		a.Logger.Error("Error creating POST request",
			slog.String("url", url),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("Error while making POST request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if len(headersInfo) > 0 {
		for _, headers := range headersInfo {
			for key, value := range headers {
				req.Header.Set(key, value)
			}
		}
	}

	resp, err := a.Client.Do(req)
	if err != nil {
		a.Logger.Error("POST request failed",
			slog.String("url", url),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("Error while making POST request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		a.Logger.Error("Error reading POST response body",
			slog.String("url", url),
			slog.Int("status", resp.StatusCode),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("error while reading response body: %w", err)
	}
	a.Logger.Debug("POST response",
		slog.String("url", url),
		slog.Int("status", resp.StatusCode),
		slog.String("request_body", string(requestBodyBytes)),
		slog.String("response_body", string(bodyBytes)),
	)

	if response != nil && resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		if err := json.Unmarshal(bodyBytes, &response); err != nil {
			a.Logger.Error("Error decoding POST response body",
				slog.String("url", url),
				slog.Int("status", resp.StatusCode),
				slog.String("error", err.Error()),
			)
			return nil, fmt.Errorf("Error while decoding response body: %w", err)
		}
	}

	a.Logger.Info("POST request successful",
		slog.String("url", url),
		slog.Int("status", resp.StatusCode),
	)

	return resp, nil
}
