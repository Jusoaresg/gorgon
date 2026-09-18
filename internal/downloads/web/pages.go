package web

import (
	"log/slog"
	"net/http"

	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	qbittorrentService "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	"github.com/jusoaresg/gorgon/internal/downloads/service"
	episodeEvents "github.com/jusoaresg/gorgon/internal/episode/events"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	epContentModel "github.com/jusoaresg/gorgon/internal/episode_content/model"
	epContentRepository "github.com/jusoaresg/gorgon/internal/episode_content/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/jusoaresg/gorgon/utils"
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

func (h *Handler) RemoveDownload(c echo.Context) error {
	logger := config.GetLogger()

	hash := c.FormValue("hash")
	if hash == "" {
		schemas.SendError(c, 400, "Missing torrent hash")
		return nil
	}

	torrentService, err := qbittorrentService.NewQBittorrentService(logger)
	if err != nil {
		logger.Error("failed to create qbittorrent service for removing download", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Torrent client not available")
		return nil
	}

	if err := torrentService.DeleteTorrent(hash, true); err != nil {
		logger.Error("failed to delete torrent from client", slog.String("hash", hash), slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Failed to remove torrent")
		return nil
	}

	episodeTorrent, err := h.EpisodeTorrentRepo.GetByHash(hash)
	if err == nil && episodeTorrent.EpisodeId != 0 {
		if err := h.EpisodeTorrentRepo.DeleteByEpisodeID(episodeTorrent.EpisodeId); err != nil {
			logger.Error("failed to delete episode torrent", slog.String("error", err.Error()))
		}

		episode, err := h.EpisodeRepo.GetByID(episodeTorrent.EpisodeId)
		if err == nil {
			cfg, err := config.LoadConfig()
			if err == nil {
				showRepo := showRepository.NewShowRepository(h.DB)
				show, err := showRepo.GetById(episode.ShowID)
				if err == nil {
					epContentRepo := epContentRepository.NewEpisodeContentRepository(h.DB)
					episodeContents, err := epContentRepo.ListByEpisodeId(episode.ID)
					if err == nil {
						for _, ec := range episodeContents {
							_ = utils.DeleteSymlink(cfg.ShowsFolder, show.Name, episode, ec)
							_ = epContentRepo.DeleteById(ec.ID)
						}
					}
					_ = utils.DeleteSymlink(cfg.ShowsFolder, show.Name, episode, epContentModel.EpisodeContent{})
					_ = utils.CleanBrokenSymlinksInSeason(cfg.ShowsFolder, show.Name, episode.Season)
				}
			}

			episode.Tracking = episodeModel.TrackingSkipped
			if err := h.EpisodeRepo.Update(episode); err != nil {
				logger.Error("failed to reset episode tracking to skipped", slog.Int64("episode_id", episode.ID), slog.String("error", err.Error()))
			} else {
				episodeEvents.EmitEpisodeTrackingUpdatedEvent(episode.ID, episode.Tracking, "")
			}
		}
	}

	schemas.SendSuccess(c, "Remove Download", map[string]any{
		"toastMessage": "Download removed",
	})
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
