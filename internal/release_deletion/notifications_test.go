package release_deletion

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jusoaresg/gorgon/pkg/handler"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestWebSocketEventsFollowPersistedStateAndOfflineCompletionSurvives(t *testing.T) {
	s, _, _, _, deleteFiles := fixture(t)
	*deleteFiles = true
	e := echo.New()
	e.GET("/ws", handler.WebSocketHandler)
	server := httptest.NewServer(e)
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	require.NoError(t, err)
	s.Options.OnChange = func(op Operation) {
		stored, err := s.Get(op.ID)
		require.NoError(t, err)
		require.Equal(t, op.Version, stored.Version)
		require.Equal(t, op.Status, stored.Status)
	}
	op, err := s.Submit(context.Background(), Request{EpisodeID: 1, Intention: "skipped"})
	require.NoError(t, err)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	var event struct {
		Type      string    `json:"type"`
		Operation Operation `json:"operation"`
	}
	require.NoError(t, conn.ReadJSON(&event))
	require.Equal(t, "ReleaseDeletionUpdated", event.Type)
	require.Equal(t, op.ID, event.Operation.ID)
	require.Equal(t, "pending", event.Operation.Status)
	require.NoError(t, conn.Close()) // Failure to deliver future events cannot undo cleanup.
	require.NoError(t, s.RecoverDue(context.Background(), time.Now()))
	final, err := s.Get(op.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", final.Status)
	require.Greater(t, final.Version, event.Operation.Version)
}
