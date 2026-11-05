package service

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	"github.com/jusoaresg/gorgon/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger() *slog.Logger {
	return slog.Default()
}

func newTestService(serverURL string, logger *slog.Logger) *QBittorrentService {
	host := strings.TrimPrefix(serverURL, "http://")
	return &QBittorrentService{
		APIService: services.NewAPIService(serverURL, logger),
		Logger:     logger,
		username:   "admin",
		password:   "adminadmin",
		host:       "http://" + host,
		port:       strings.TrimPrefix(host, "127.0.0.1:"),
	}
}

func TestLogin_Success(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/auth/login", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		r.ParseForm()
		assert.Equal(t, "admin", r.FormValue("username"))
		assert.Equal(t, "adminadmin", r.FormValue("password"))

		http.SetCookie(w, &http.Cookie{
			Name:  "QBT_SID_9191",
			Value: "test-sid-value",
			Path:  "/",
		})
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	err := svc.Login(&schema.QBittorrentLoginRequest{
		Username: "admin",
		Password: "adminadmin",
	})

	require.NoError(t, err)
	assert.Equal(t, "test-sid-value", svc.sid)
	assert.Equal(t, "QBT_SID_9191", svc.cookieName)
	assert.True(t, svc.IsAuthenticated())
}

func TestLogin_Forbidden(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Forbidden."))
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	err := svc.Login(&schema.QBittorrentLoginRequest{
		Username: "admin",
		Password: "wrong",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid credentials")
}

func TestLogin_ClassicSID(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:  "SID",
			Value: "classic-sid",
			Path:  "/",
		})
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	err := svc.Login(&schema.QBittorrentLoginRequest{
		Username: "admin",
		Password: "adminadmin",
	})

	require.NoError(t, err)
	assert.Equal(t, "classic-sid", svc.sid)
	assert.Equal(t, "SID", svc.cookieName)
}

func TestSidVerification_AlreadyLoggedIn(t *testing.T) {
	logger := newTestLogger()

	svc := newTestService("", logger)
	svc.sid = "test-sid"
	svc.cookieName = "QBT_SID_9191"

	err := svc.EnsureAuthenticated()

	require.NoError(t, err)
}

func TestSidVerification_NeedsLogin(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			http.SetCookie(w, &http.Cookie{
				Name:  "QBT_SID_9191",
				Value: "fresh-sid",
				Path:  "/",
			})
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)

	err := svc.EnsureAuthenticated()
	require.NoError(t, err)
	assert.True(t, svc.IsAuthenticated())
}

func TestCookieHeader(t *testing.T) {
	svc := &QBittorrentService{
		cookieName: "QBT_SID_9191",
		sid:        "my-sid",
	}
	assert.Equal(t, "QBT_SID_9191=my-sid", svc.cookieHeader())

	svc2 := &QBittorrentService{
		cookieName: "SID",
		sid:        "classic",
	}
	assert.Equal(t, "SID=classic", svc2.cookieHeader())

	svc3 := &QBittorrentService{
		sid: "no-name",
	}
	assert.Equal(t, "SID=no-name", svc3.cookieHeader())
}

func TestAddTorrent_Success(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "QBT_SID_9191", Value: "test-sid", Path: "/"})
			w.WriteHeader(http.StatusNoContent)
		case "/api/v2/torrents/add":
			assert.Equal(t, "POST", r.Method)
			assert.Contains(t, r.Header.Get("Cookie"), "QBT_SID_9191=test-sid")

			r.ParseMultipartForm(10 << 20)
			assert.Equal(t, "magnet:?xt=urn:btih:test123", r.FormValue("urls"))
			assert.Equal(t, "gorgon", r.FormValue("category"))
			assert.Equal(t, "false", r.FormValue("paused"))

			resp := addTorrentResponse{
				AddedTorrentIds: []string{"test123"},
				FailureCount:    0,
				SuccessCount:    1,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	err := svc.AddTorrent("magnet:?xt=urn:btih:test123")
	require.NoError(t, err)
}

func TestAddTorrent_Failure(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "QBT_SID_9191", Value: "test-sid", Path: "/"})
			w.WriteHeader(http.StatusNoContent)
		case "/api/v2/torrents/add":
			resp := addTorrentResponse{
				AddedTorrentIds: []string{},
				FailureCount:    1,
				SuccessCount:    0,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	err := svc.AddTorrent("magnet:?xt=urn:btih:bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failures=1")
}

func TestAddTorrent_AlreadyExists(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "QBT_SID_9191", Value: "test-sid", Path: "/"})
			w.WriteHeader(http.StatusNoContent)
		case "/api/v2/torrents/add":
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte("Conflict"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	err := svc.AddTorrent("magnet:?xt=urn:btih:already")
	require.NoError(t, err)
}

func TestDeleteTorrent_Success(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "QBT_SID_9191", Value: "test-sid", Path: "/"})
			w.WriteHeader(http.StatusNoContent)
		case "/api/v2/torrents/delete":
			assert.Equal(t, "POST", r.Method)
			r.ParseForm()
			assert.Equal(t, "abc123", r.FormValue("hashes"))
			assert.Equal(t, "true", r.FormValue("deleteFiles"))
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	err := svc.DeleteTorrent("abc123", true)
	require.NoError(t, err)
}

func TestDeleteTorrent_WrongMethod(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "QBT_SID_9191", Value: "test-sid", Path: "/"})
			w.WriteHeader(http.StatusNoContent)
		case "/api/v2/torrents/delete":
			if r.Method == "GET" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				w.Write([]byte("Method Not Allowed"))
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	err := svc.DeleteTorrent("abc123", false)
	require.NoError(t, err)
}

func TestCheckTorrents_Success(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "QBT_SID_9191", Value: "test-sid", Path: "/"})
			w.WriteHeader(http.StatusNoContent)
		case "/api/v2/torrents/info":
			assert.Equal(t, "GET", r.Method)
			assert.Equal(t, "downloading", r.URL.Query().Get("filter"))
			assert.Contains(t, r.Header.Get("Cookie"), "QBT_SID_9191=test-sid")

			torrents := []schema.CheckTorrentResponse{
				{
					Name:     "test-torrent",
					Progress: 0.5,
					State:    "downloading",
					Hash:     "abc123",
					DlSpeed:  1024000,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(torrents)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	var result []schema.CheckTorrentResponse
	err := svc.CheckTorrents("downloading", &result)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "test-torrent", result[0].Name)
	assert.Equal(t, float32(0.5), result[0].Progress)
}

func TestCheckTorrentsWithHash_Success(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "QBT_SID_9191", Value: "test-sid", Path: "/"})
			w.WriteHeader(http.StatusNoContent)
		case "/api/v2/torrents/info":
			assert.Equal(t, "all", r.URL.Query().Get("filter"))
			assert.Equal(t, "abc123hash", r.URL.Query().Get("hashes"))

			torrents := []schema.CheckTorrentResponse{
				{Name: "specific-torrent", Hash: "abc123hash", Progress: 1.0},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(torrents)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	var result []schema.CheckTorrentResponse
	err := svc.CheckTorrentsWithHash("all", "abc123hash", &result)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "specific-torrent", result[0].Name)
}

func TestCheckConnection_Success(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/app/version" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("v4.4.0"))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "QBT_SID_9191", Value: "test-sid", Path: "/"})
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	err := svc.CheckConnection()
	require.NoError(t, err)
}

func TestCheckConnection_Failure(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Forbidden."))
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	err := svc.CheckConnection()
	require.Error(t, err)
}

func TestLogin_NoContentWithoutSIDCookie(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	err := svc.Login(&schema.QBittorrentLoginRequest{
		Username: "admin",
		Password: "adminadmin",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "SID not found")
	assert.False(t, svc.IsAuthenticated())
}

func TestLogin_SuccessBodyWithoutCookie(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ok."))
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	err := svc.Login(&schema.QBittorrentLoginRequest{
		Username: "admin",
		Password: "adminadmin",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "SID not found")
}

func TestLogin_ForbiddenBanned(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Your IP address has been banned after too many failed authentication attempts."))
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	err := svc.Login(&schema.QBittorrentLoginRequest{
		Username: "admin",
		Password: "adminadmin",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "banned")
	assert.NotContains(t, err.Error(), "invalid credentials")
}

func TestCheckConnection_PersistentForbiddenIncludesBody(t *testing.T) {
	logger := newTestLogger()
	var logins atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			logins.Add(1)
			http.SetCookie(w, &http.Cookie{Name: fmt.Sprintf("QBT_SID_8080_%d", logins.Load()), Value: "sid", Path: "/"})
			w.WriteHeader(http.StatusNoContent)
		case "/api/v2/app/version":
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Your IP address has been banned after too many failed authentication attempts."))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	err := svc.CheckConnection()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status=403")
	assert.Contains(t, err.Error(), "banned")
	assert.Equal(t, int32(2), logins.Load(), "should have re-authenticated once before failing")
}

func TestSessionExpiry_ReauthRecovers(t *testing.T) {
	logger := newTestLogger()
	var versionCalls atomic.Int32
	var sidCounter atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "QBT_SID_8080", Value: fmt.Sprintf("sid-%d", sidCounter.Add(1)), Path: "/"})
			w.WriteHeader(http.StatusNoContent)
		case "/api/v2/torrents/info":
			if versionCalls.Add(1) == 1 {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte("Forbidden"))
				return
			}
			assert.Contains(t, r.Header.Get("Cookie"), "sid-2", "retry must use the fresh SID")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]schema.CheckTorrentResponse{{Name: "ok", Hash: "h1"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	var result []schema.CheckTorrentResponse
	err := svc.CheckTorrents("all", &result)

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, int32(2), sidCounter.Load(), "a new session should have been issued")
}

func TestGetContent_Success(t *testing.T) {
	logger := newTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "QBT_SID_8080", Value: "test-sid", Path: "/"})
			w.WriteHeader(http.StatusNoContent)
		case "/api/v2/torrents/files":
			assert.Contains(t, r.Header.Get("Cookie"), "QBT_SID_8080=test-sid")
			assert.Equal(t, "abc123", r.URL.Query().Get("hash"))
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]any{
				{"name": "S01E01.mkv", "size": 1000},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := newTestService(server.URL, logger)
	content, err := svc.GetContent("abc123")

	require.NoError(t, err)
	require.Len(t, content, 1)
	assert.Equal(t, "S01E01.mkv", content[0].Name)
	assert.Equal(t, float64(1000), content[0].Size)
}

func TestAddTorrentIfAbsentDoesNotOwnExistingTorrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test"})
			fmt.Fprint(w, "Ok.")
			return
		}
		w.WriteHeader(http.StatusConflict)
	}))
	defer server.Close()
	logger := slog.Default()
	svc := &QBittorrentService{APIService: services.NewAPIService(server.URL, logger), Logger: logger}
	owned, err := svc.AddTorrentIfAbsent("magnet:?xt=urn:btih:existing")
	assert.NoError(t, err)
	assert.False(t, owned)
}
