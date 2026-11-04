package web

import (
	"errors"
	"github.com/jusoaresg/gorgon/internal/release_deletion"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	qbittorrentService "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	"github.com/jusoaresg/gorgon/internal/downloads/service"
	episodeEvents "github.com/jusoaresg/gorgon/internal/episode/events"
	"github.com/jusoaresg/gorgon/internal/scheduler/jobs"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/jusoaresg/gorgon/views"
	"github.com/labstack/echo/v4"
)

type DownloadsData struct {
	Items        []service.DownloadItem
	Summary      service.DownloadsSummary
	ErrorMessage string
}

func (h *Handler) DownloadsRoute(c echo.Context) error {
	items, summary, err := h.fetchDownloadItems()
	errorMessage := ""
	if err != nil {
		errorMessage = "Unable to reach the torrent client. Configure qBittorrent in Settings."
	}

	return views.Render(c, views.View{
		Layout:    "layout",
		Default:   "downloads",
		Templates: map[string]string{"download-items": "download-items"},
		Data:      DownloadsData{Items: items, Summary: summary, ErrorMessage: errorMessage},
		Styles:    []string{"downloads.css"},
	})
}

func (h *Handler) DownloadsItemsHTMX(c echo.Context) error {
	items, summary, err := h.fetchDownloadItems()
	if err != nil {
		return c.Render(http.StatusOK, "download-items", views.PageData{Data: DownloadsData{Items: []service.DownloadItem{}}})
	}

	return c.Render(http.StatusOK, "download-items", views.PageData{Data: DownloadsData{Items: items, Summary: summary}})
}

func (h *Handler) PauseDownload(c echo.Context) error {
	logger := config.GetLogger()

	hash := c.FormValue("hash")
	if hash == "" {
		schemas.SendError(c, 400, "Missing torrent hash")
		return nil
	}

	torrentService, err := qbittorrentService.NewQBittorrentService(logger)
	if err != nil {
		logger.Error("failed to create qbittorrent service for pausing download", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Torrent client not available")
		return nil
	}

	if err := torrentService.PauseTorrent(hash); err != nil {
		logger.Error("failed to pause torrent in client", slog.String("hash", hash), slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Failed to pause download")
		return nil
	}

	schemas.SendSuccess(c, "Pause Download", map[string]any{
		"toastMessage": "Download paused",
	})
	return nil
}

func (h *Handler) ResumeDownload(c echo.Context) error {
	logger := config.GetLogger()

	hash := c.FormValue("hash")
	if hash == "" {
		schemas.SendError(c, 400, "Missing torrent hash")
		return nil
	}

	torrentService, err := qbittorrentService.NewQBittorrentService(logger)
	if err != nil {
		logger.Error("failed to create qbittorrent service for resuming download", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Torrent client not available")
		return nil
	}

	if err := torrentService.ResumeTorrent(hash); err != nil {
		logger.Error("failed to resume torrent in client", slog.String("hash", hash), slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Failed to resume download")
		return nil
	}

	schemas.SendSuccess(c, "Resume Download", map[string]any{
		"toastMessage": "Download resumed",
	})
	return nil
}

func (h *Handler) ImportDownload(c echo.Context) error {
	logger := config.GetLogger()

	hash := c.FormValue("hash")
	if hash == "" {
		schemas.SendError(c, 400, "Missing torrent hash")
		return nil
	}

	episodeTorrent, err := h.EpisodeTorrentRepo.GetByHash(hash)
	if err != nil || episodeTorrent.EpisodeId == 0 {
		logger.Error("no episode torrent found for import", slog.String("hash", hash))
		schemas.SendError(c, 404, "No tracked episode found for this download")
		return nil
	}

	episode, err := h.EpisodeRepo.GetByID(episodeTorrent.EpisodeId)
	if err != nil {
		logger.Error("failed to get episode for import", slog.Int64("episode_id", episodeTorrent.EpisodeId), slog.String("error", err.Error()))
		schemas.SendError(c, 404, "Episode not found")
		return nil
	}

	torrentService, err := qbittorrentService.NewQBittorrentService(logger)
	if err != nil {
		logger.Error("failed to create qbittorrent service for importing download", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Torrent client not available")
		return nil
	}

	if err := jobs.ProcessSingleSnatchedDownload(&episode, torrentService); err != nil {
		logger.Error("failed to process single snatched download manually", slog.String("hash", hash), slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Failed to import episode")
		return nil
	}

	schemas.SendSuccess(c, "Import Download", map[string]any{
		"toastMessage": "Episode imported successfully",
	})
	return nil
}

// RemoveDownload delegates all physical cleanup and finalization to the shared coordinator.
func (h *Handler) RemoveDownload(c echo.Context) error {
	hash := strings.ToLower(strings.TrimSpace(c.FormValue("hash")))
	if hash == "" {
		schemas.SendError(c, http.StatusBadRequest, "Missing torrent hash")
		return nil
	}
	svc := h.DeletionService
	if svc == nil {
		var err error
		svc, err = release_deletion.New(h.DB, config.GetLogger())
		if err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]any{"error": "Torrent client is unavailable; download was preserved"})
		}
	}
	op, err := svc.Delete(c.Request().Context(), release_deletion.Request{Hash: hash, Intention: "skipped"})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, release_deletion.ErrConflict) {
			status = http.StatusConflict
		}
		if errors.Is(err, release_deletion.ErrUnsafe) {
			status = http.StatusUnprocessableEntity
		}
		return c.JSON(status, map[string]any{"operation": op, "error": "Download deletion is incomplete; recovery context was preserved"})
	}
	if op.Status != "completed" {
		return c.JSON(http.StatusAccepted, map[string]any{"operation": op, "message": "Download deletion is in progress"})
	}
	schemas.SendSuccess(c, "Remove Download", map[string]any{"toastMessage": "Download removed", "operation": op})
	return nil
}

func (h *Handler) BlacklistDownload(c echo.Context) error {
	hash := strings.ToLower(strings.TrimSpace(c.FormValue("hash")))
	if hash == "" {
		schemas.SendError(c, http.StatusBadRequest, "Missing torrent hash")
		return nil
	}
	svc := h.DeletionService
	if svc == nil {
		var err error
		svc, err = release_deletion.New(h.DB, config.GetLogger())
		if err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]any{"error": "Torrent client is unavailable; release was preserved"})
		}
	}
	op, err := svc.Delete(c.Request().Context(), release_deletion.Request{Hash: hash, Intention: "blacklist", BlacklistReason: "manual blacklist"})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, release_deletion.ErrConflict) {
			status = http.StatusConflict
		}
		if errors.Is(err, release_deletion.ErrUnsafe) {
			status = http.StatusUnprocessableEntity
		}
		return c.JSON(status, map[string]any{"operation": op, "error": "Release cleanup is incomplete; replacement search has not started"})
	}
	if op.Status != "completed" {
		return c.JSON(http.StatusAccepted, map[string]any{"operation": op, "message": "Blacklist cleanup is in progress"})
	}
	schemas.SendSuccess(c, "Blacklist Download", map[string]any{"toastMessage": "Release blacklisted; replacement search queued", "operation": op})
	return nil
}

func (h *Handler) fetchDownloadItems() ([]service.DownloadItem, service.DownloadsSummary, error) {
	logger := config.GetLogger()

	torrentService, err := qbittorrentService.NewQBittorrentService(logger)
	if err != nil {
		logger.Warn("failed to create qbittorrent service for downloads page", slog.String("error", err.Error()))
		return []service.DownloadItem{}, service.DownloadsSummary{}, err
	}

	var torrents []schema.CheckTorrentResponse
	if err := torrentService.CheckTorrentsWithCategory("all", "gorgon", &torrents); err != nil {
		logger.Warn("failed to fetch torrents for downloads page", slog.String("error", err.Error()))
		return []service.DownloadItem{}, service.DownloadsSummary{}, err
	}

	items, err := h.Service.BuildDownloads(torrents)
	if err != nil {
		return []service.DownloadItem{}, service.DownloadsSummary{}, err
	}

	summary := h.Service.ComputeSummary(items)
	return items, summary, nil
}
