package handler

import (
	"github.com/jusoaresg/gorgon/config"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	_ "github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var clients = make(map[*websocket.Conn]bool)
var mutex = &sync.Mutex{}

func WebSocketHandler(c echo.Context) error {
	logger := config.GetLogger().WithGroup("websocket").With("name", "WebSocketHandler")
	conn, err := upgrader.Upgrade(c.Response().Writer, c.Request(), nil)
	if err != nil {
		//TODO: Logger message
		logger.Error("webSocket upgrade failed", slog.String("error", err.Error()))
		return err
	}

	mutex.Lock()
	clients[conn] = true
	mutex.Unlock()

	defer func() {
		mutex.Lock()
		delete(clients, conn)
		mutex.Unlock()
		conn.Close()
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	return nil
}

var broadcastQueue = make(chan any, 128)
var broadcasterOnce sync.Once

// SendWebSocketMessage never makes application state transitions wait for clients.
// Slow/offline clients recover durable operation state through the query API.
func SendWebSocketMessage(message any) {
	broadcasterOnce.Do(func() {
		go func() {
			for item := range broadcastQueue {
				broadcast(item)
			}
		}()
	})
	select {
	case broadcastQueue <- message:
	default:
	}
}
func broadcast(message any) {
	logger := config.GetLogger().WithGroup("websocket").With("name", "SendWebSocketMessage")
	mutex.Lock()
	defer mutex.Unlock()

	deadline := time.Now().Add(250 * time.Millisecond)
	for client := range clients {
		if time.Now().After(deadline) {
			break
		}
		_ = client.SetWriteDeadline(deadline)
		if err := client.WriteJSON(message); err != nil {
			logger.Error("error while sending websocket message", slog.Any("Message", message), slog.Any("Client", client))
			client.Close()
			delete(clients, client)
		}
	}
}
