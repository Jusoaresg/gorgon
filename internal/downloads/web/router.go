package web

import (
	"github.com/jusoaresg/gorgon/internal/downloads"
	"github.com/labstack/echo/v4"
)

func RegisterDownloadsRoutes(e *echo.Echo, deps *downloads.Dependencies) {
	handler := NewHandler(deps)

	e.GET("/downloads", handler.DownloadsRoute)

	front := e.Group("/front/")
	front.GET("downloads/items", handler.DownloadsItemsHTMX)
	front.POST("downloads/remove", handler.RemoveDownload)
	front.POST("downloads/pause", handler.PauseDownload)
	front.POST("downloads/resume", handler.ResumeDownload)
}
