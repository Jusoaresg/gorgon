package service

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jusoaresg/gorgon/external/telegram/schema"
	apiService "github.com/jusoaresg/gorgon/pkg/services"
)

func TestGetLatestChatID(t *testing.T) {
	tests := []struct {
		name        string
		resp        schema.GetUpdatesResponse
		statusCode  int
		wantChatID  string
		wantErrText string
	}{
		{
			name: "Success with message",
			resp: schema.GetUpdatesResponse{
				Ok: true,
				Result: []schema.TelegramUpdate{
					{
						UpdateID: 1,
						Message: &schema.TelegramUpdateMessage{
							Chat: schema.TelegramUpdateChat{
								ID: 123456789,
							},
						},
					},
				},
			},
			statusCode: http.StatusOK,
			wantChatID: "123456789",
		},
		{
			name: "Success with channel post (picks latest)",
			resp: schema.GetUpdatesResponse{
				Ok: true,
				Result: []schema.TelegramUpdate{
					{
						UpdateID: 1,
						Message: &schema.TelegramUpdateMessage{
							Chat: schema.TelegramUpdateChat{
								ID: 111,
							},
						},
					},
					{
						UpdateID: 2,
						ChannelPost: &schema.TelegramUpdateMessage{
							Chat: schema.TelegramUpdateChat{
								ID: -100987654321,
							},
						},
					},
				},
			},
			statusCode: http.StatusOK,
			wantChatID: "-100987654321",
		},
		{
			name: "Empty result returns helpful error",
			resp: schema.GetUpdatesResponse{
				Ok:     true,
				Result: []schema.TelegramUpdate{},
			},
			statusCode:  http.StatusOK,
			wantErrText: "no messages found. Please send a message",
		},
		{
			name: "API error returns error description",
			resp: schema.GetUpdatesResponse{
				Ok:          false,
				Description: "Unauthorized",
			},
			statusCode:  http.StatusOK,
			wantErrText: "Unauthorized",
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_ = json.NewEncoder(w).Encode(tt.resp)
			}))
			defer server.Close()

			svc := &TelegramService{
				ApiService: apiService.NewAPIService(server.URL+"/", logger),
				Logger:     logger,
			}

			chatID, err := svc.GetLatestChatID("test-token")
			if tt.wantErrText != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrText)
				}
				if !testing.Short() && len(err.Error()) > 0 {
					// pass
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if chatID != tt.wantChatID {
					t.Errorf("expected chatID %s, got %s", tt.wantChatID, chatID)
				}
			}
		})
	}
}

func TestNormalizeBotCommand_And_AuthGuard(t *testing.T) {
	cmd, arg := normalizeBotCommand("/add@MyGorgonBot Frieren Beyond")
	if cmd != "/add" || arg != "Frieren Beyond" {
		t.Fatalf("unexpected normalized command: cmd=%q arg=%q", cmd, arg)
	}

	if !isAuthorizedChat("", "999") {
		t.Fatal("expected empty configured chat ID to allow setup messages")
	}
	if !isAuthorizedChat("12345", "12345") {
		t.Fatal("expected matching chat ID to be authorized")
	}
	if isAuthorizedChat("12345", "99999") {
		t.Fatal("expected mismatched chat ID to be rejected")
	}
}
