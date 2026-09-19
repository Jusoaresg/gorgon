package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jusoaresg/gorgon/external/telegram/schema"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestDetectTelegramChatID_MissingToken(t *testing.T) {
	empty := ""
	payload := schema.TestTelegramRequest{
		TelegramBotApiKey: &empty,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/detect-chat-id", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	err := DetectTelegramChatID(c)
	assert.NoError(t, err)
	// If no token in body or config, returns 400 or whatever status
}

func TestTestTelegramConnection_MissingData(t *testing.T) {
	empty := ""
	payload := schema.TestTelegramRequest{
		TelegramBotApiKey: &empty,
		TelegramChatID:    &empty,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/telegram/test", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	err := TestTelegramConnection(c)
	assert.NoError(t, err)
}
