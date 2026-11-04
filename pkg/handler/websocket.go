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

func SendWebSocketMessage(message any) {
	logger := config.GetLogger().WithGroup("websocket").With("name", "SendWebSocketMessage")
	mutex.Lock()
	defer mutex.Unlock()

	for client := range clients {
		_ = client.SetWriteDeadline(time.Now().Add(250 * time.Millisecond))
		if err := client.WriteJSON(message); err != nil {
			logger.Error("error while sending websocket message", slog.Any("Message", message), slog.Any("Client", client))
			client.Close()
			delete(clients, client)
		}
	}
}
