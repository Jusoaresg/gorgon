package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	"github.com/jusoaresg/gorgon/internal/episode_content/model"
	"github.com/jusoaresg/gorgon/pkg/services"
)

type ErrQBittorrentHostPortNotSet interface {
	Error() string
	IsProwlarrConfigWarn() bool
}
type QBittorrentHostPortNotSet struct{}

func (m *QBittorrentHostPortNotSet) Error() string {
	return "QBittorrent Host or Port not set"
}

func (m *QBittorrentHostPortNotSet) IsProwlarrConfigWarn() bool {
	return true
}

type addTorrentResponse struct {
	AddedTorrentIds []string `json:"added_torrent_ids"`
	FailureCount    int      `json:"failure_count"`
	PendingCount    int      `json:"pending_count"`
	SuccessCount    int      `json:"success_count"`
}

type QBittorrentService struct {
	APIService *services.APIService
	Logger     *slog.Logger

	username       string
	password       string
	host           string
	port           string
	DownloadFolder string

	mu         sync.Mutex
	sid        string
	cookieName string
}

func NewQBittorrentService(logger *slog.Logger) (*QBittorrentService, error) {
	configFile, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}
	host := configFile.QBittorrentHost
	port := configFile.QBittorrentPort

	if host == "" || port == "" {
		return nil, &QBittorrentHostPortNotSet{}
	}

	downloadFolder := configFile.QBittorrentDownloadFolder

	return &QBittorrentService{
		APIService:     services.NewAPIService(fmt.Sprintf("%s:%s", host, port), logger),
		Logger:         logger,
		username:       configFile.QBittorrentUsername,
		password:       configFile.QBittorrentPassword,
		DownloadFolder: downloadFolder,
		host:           host,
		port:           port,
	}, nil
}

func (q *QBittorrentService) Name() string {
	return "qBittorrent"
}

func (q *QBittorrentService) isAuthenticated() bool {
	return q.sid != ""
}

func (q *QBittorrentService) IsAuthenticated() bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.isAuthenticated()
}

func (q *QBittorrentService) Login(request *schema.QBittorrentLoginRequest) error {
	form := url.Values{}
	form.Add("username", request.Username)
	form.Add("password", request.Password)

	headers := map[string]string{
		"Referer": fmt.Sprintf("%s:%s", q.host, q.port),
	}

	resp, err := q.APIService.PostRaw(
		"/api/v2/auth/login",
		"application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()),
		headers,
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return q.loginForbiddenError(resp)
	}

	sid, cookieName, ok := extractSessionCookie(resp)
	if !ok {
		return fmt.Errorf("QBittorrent SID not found (status=%d)", resp.StatusCode)
	}

	q.Logger.Debug("qbittorrent login successful",
		slog.Int("status", resp.StatusCode),
		slog.String("cookie_name", cookieName),
		slog.Int("sid_length", len(sid)),
	)

	q.sid = sid
	q.cookieName = cookieName
	return nil
}

// extractSessionCookie finds the session cookie in a login response. Since
// qBittorrent 5.2 the cookie is named "QBT_SID_<port>" instead of "SID"; both
// forms are accepted. The cookie value must be non-empty.
func extractSessionCookie(resp *http.Response) (sid, cookieName string, ok bool) {
	for _, cookie := range resp.Cookies() {
		if cookie.Value == "" {
			continue
		}
		if strings.HasPrefix(cookie.Name, "SID") || strings.HasPrefix(cookie.Name, "QBT_SID") {
			return cookie.Value, cookie.Name, true
		}
	}
	return "", "", false
}

// loginForbiddenError builds an error for a 403 login response. qBittorrent
// returns 403 when the client IP is banned after too many failed attempts,
// which is a different problem from wrong credentials.
func (q *QBittorrentService) loginForbiddenError(resp *http.Response) error {
	snippet := responseBodySnippet(resp)
	if strings.Contains(strings.ToLower(snippet), "ban") {
		return fmt.Errorf(
			"qbittorrent refused login: this IP is temporarily banned after too many failed authentication attempts; wait for the ban to expire or restart qbittorrent (body=%q)",
			snippet,
		)
	}
	if snippet == "" {
		return fmt.Errorf("invalid credentials")
	}
	return fmt.Errorf("invalid credentials (body=%q)", snippet)
}

func (q *QBittorrentService) EnsureAuthenticated() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.isAuthenticated() {
		return nil
	}

	req := schema.QBittorrentLoginRequest{
		Username: q.username,
		Password: q.password,
	}

	if err := q.Login(&req); err != nil {
		return fmt.Errorf("error while logging on qbittorrent: %w", err)
	}

	return nil
}

func (q *QBittorrentService) Reauthenticate(oldSID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.sid != oldSID {
		return nil
	}

	q.sid = ""

	req := schema.QBittorrentLoginRequest{
		Username: q.username,
		Password: q.password,
	}
	return q.Login(&req)
}

func (q *QBittorrentService) cookieHeader() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.cookieName != "" {
		return fmt.Sprintf("%s=%s", q.cookieName, q.sid)
	}
	return fmt.Sprintf("SID=%s", q.sid)
}

func (q *QBittorrentService) currentSID() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.sid
}

// responseBodySnippet reads up to 512 bytes of an error response body so the
// reason sent by qbittorrent (ban message, validation error, etc.) shows up
// in logs instead of a bare status code.
func responseBodySnippet(resp *http.Response) string {
	if resp.Body == nil {
		return ""
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return strings.TrimSpace(string(body))
}

func (q *QBittorrentService) CheckConnection() error {
	resp, err := q.doAuthenticatedRequest(func() (*http.Response, error) {
		headers := map[string]string{
			"Cookie":  q.cookieHeader(),
			"Referer": fmt.Sprintf("%s:%s", q.host, q.port),
		}
		return q.APIService.GetWithHeadersRaw("/api/v2/app/version", headers)
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"qbittorrent connection check failed (status=%d): %s",
			resp.StatusCode,
			responseBodySnippet(resp),
		)
	}
	return nil
}

func (q *QBittorrentService) Logout() error {
	return nil
}

// doAuthenticatedRequest runs an authenticated API call. When the call comes
// back with 403 (expired/invalidated session), it re-authenticates once and
// retries. The request func must return the raw response (it may hold a
// non-OK status); converting statuses into domain errors is done by the
// callers afterwards, otherwise the retry would never be reached.
func (q *QBittorrentService) doAuthenticatedRequest(request func() (*http.Response, error)) (*http.Response, error) {
	if err := q.EnsureAuthenticated(); err != nil {
		return nil, err
	}

	resp, err := request()
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusForbidden {
		return resp, nil
	}

	oldSID := q.currentSID()
	resp.Body.Close()

	if err := q.Reauthenticate(oldSID); err != nil {
		return nil, err
	}

	resp, err = request()
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusForbidden {
		defer resp.Body.Close()
		return nil, fmt.Errorf(
			"qbittorrent request failed after re-authentication (status=%d): %s",
			resp.StatusCode,
			responseBodySnippet(resp),
		)
	}

	return resp, nil
}

func (q *QBittorrentService) AddTorrent(torrentUrl string) error {
	var addResp addTorrentResponse

	resp, err := q.doAuthenticatedRequest(func() (*http.Response, error) {
		var requestBody bytes.Buffer
		writer := multipart.NewWriter(&requestBody)

		writer.WriteField("urls", torrentUrl)
		writer.WriteField("category", "gorgon")
		writer.WriteField("skip_checking", "false")
		writer.WriteField("root_folder", "false")
		writer.WriteField("paused", "false")
		writer.Close()

		headers := map[string]string{
			"Content-Type": writer.FormDataContentType(),
			"Cookie":       q.cookieHeader(),
			"Referer":      fmt.Sprintf("%s:%s", q.host, q.port),
		}

		return q.APIService.Post("/api/v2/torrents/add", requestBody.Bytes(), &addResp, headers)
	})
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusConflict {
		q.Logger.Info("torrent already exists in torrent client", slog.String("url", torrentUrl))
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to add torrent (status=%d)", resp.StatusCode)
	}

	if addResp.FailureCount > 0 {
		return fmt.Errorf("failed to add torrent (failures=%d)", addResp.FailureCount)
	}

	return nil
}

func (q *QBittorrentService) DeleteTorrent(hash string, deleteFile bool) error {
	resp, err := q.doAuthenticatedRequest(func() (*http.Response, error) {
		form := url.Values{}
		form.Add("hashes", hash)
		form.Add("deleteFiles", fmt.Sprintf("%t", deleteFile))

		headers := map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
			"Cookie":       q.cookieHeader(),
			"Referer":      fmt.Sprintf("%s:%s", q.host, q.port),
		}

		return q.APIService.Post("/api/v2/torrents/delete", form.Encode(), nil, headers)
	})
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete torrent (status=%d)", resp.StatusCode)
	}

	return nil
}

func (q *QBittorrentService) PauseTorrent(hash string) error {
	resp, err := q.doAuthenticatedRequest(func() (*http.Response, error) {
		form := url.Values{}
		form.Add("hashes", hash)

		headers := map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
			"Cookie":       q.cookieHeader(),
			"Referer":      fmt.Sprintf("%s:%s", q.host, q.port),
		}

		resp, err := q.APIService.PostRaw("/api/v2/torrents/pause", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()), headers)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			return q.APIService.PostRaw("/api/v2/torrents/stop", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()), headers)
		}
		return resp, nil
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to pause torrent (status=%d): %s", resp.StatusCode, responseBodySnippet(resp))
	}

	return nil
}

func (q *QBittorrentService) ResumeTorrent(hash string) error {
	resp, err := q.doAuthenticatedRequest(func() (*http.Response, error) {
		form := url.Values{}
		form.Add("hashes", hash)

		headers := map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
			"Cookie":       q.cookieHeader(),
			"Referer":      fmt.Sprintf("%s:%s", q.host, q.port),
		}

		resp, err := q.APIService.PostRaw("/api/v2/torrents/resume", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()), headers)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			return q.APIService.PostRaw("/api/v2/torrents/start", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()), headers)
		}
		return resp, nil
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to resume torrent (status=%d): %s", resp.StatusCode, responseBodySnippet(resp))
	}

	return nil
}

func (q *QBittorrentService) CheckTorrents(filter string, response *[]schema.CheckTorrentResponse) error {
	return q.CheckTorrentsWithCategory(filter, "", response)
}

func (q *QBittorrentService) CheckTorrentsWithCategory(filter, category string, response *[]schema.CheckTorrentResponse) error {
	resp, err := q.doAuthenticatedRequest(func() (*http.Response, error) {
		headers := map[string]string{
			"Cookie":  q.cookieHeader(),
			"Referer": fmt.Sprintf("%s:%s", q.host, q.port),
		}

		params := url.Values{}
		if filter != "" {
			params.Add("filter", filter)
		}
		if category != "" {
			params.Add("category", category)
		}

		endpoint := fmt.Sprintf("/api/v2/torrents/info?%s", params.Encode())
		q.Logger.Debug("Calling QBittorrent API", slog.String("url", endpoint), slog.String("category", category))

		resp, err := q.APIService.GetWithHeadersRaw(endpoint, headers)
		if err != nil {
			return nil, fmt.Errorf("error while get torrent info: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return resp, nil
		}

		if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("error decoding torrent info: %w", err)
		}

		resp.Body.Close()

		return resp, nil
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"failed to get torrent info (status=%d): %s",
			resp.StatusCode,
			responseBodySnippet(resp),
		)
	}

	return nil
}

func (q *QBittorrentService) CheckTorrentsWithHash(filter, hash string, response *[]schema.CheckTorrentResponse) error {
	resp, err := q.doAuthenticatedRequest(func() (*http.Response, error) {
		headers := map[string]string{
			"Cookie":  q.cookieHeader(),
			"Referer": fmt.Sprintf("%s:%s", q.host, q.port),
		}

		encodedHash := url.QueryEscape(hash)
		endpoint := fmt.Sprintf("/api/v2/torrents/info?filter=%s&hashes=%s", filter, encodedHash)

		resp, err := q.APIService.GetWithHeadersRaw(endpoint, headers)
		if err != nil {
			return nil, fmt.Errorf("error while get torrent info: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return resp, nil
		}

		if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("error decoding torrent info: %w", err)
		}

		resp.Body.Close()

		return resp, nil
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"failed to get torrent info (status=%d): %s",
			resp.StatusCode,
			responseBodySnippet(resp),
		)
	}

	return nil
}

func (q *QBittorrentService) GetContent(hash string) ([]model.EpisodeContent, error) {
	resp, err := q.doAuthenticatedRequest(func() (*http.Response, error) {
		headers := map[string]string{
			"Cookie":  q.cookieHeader(),
			"Referer": fmt.Sprintf("%s:%s", q.host, q.port),
		}

		endpoint := fmt.Sprintf("/api/v2/torrents/files?hash=%s", hash)

		resp, err := q.APIService.GetWithHeadersRaw(endpoint, headers)
		if err != nil {
			return nil, err
		}

		return resp, nil
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"failed to get torrent files (status=%d): %s",
			resp.StatusCode,
			responseBodySnippet(resp),
		)
	}

	var files []schema.TorrentContent
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("error decoding torrent files: %w", err)
	}

	var content []model.EpisodeContent
	for _, f := range files {
		content = append(content, model.EpisodeContent{
			Name: f.Name,
			Size: float64(f.Size),
		})
	}

	return content, nil
}
