package api

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	epContentModel "github.com/jusoaresg/gorgon/internal/episode_content/model"
	epContentRepository "github.com/jusoaresg/gorgon/internal/episode_content/repository"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	"github.com/jusoaresg/gorgon/internal/show/model"
	"github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/jusoaresg/gorgon/utils"

	"github.com/labstack/echo/v4"
)

// @BasePath /api/v1

// @Summary Delete Show
// @Description Delete Show from list
// @Tags Database/Show
// @Produce json
// @Param id path int true "Show Id"
// @Success 200 {object} schemas.DefaultResponse
// @Failure 400 {object} schemas.ErrorResponse
// @Failure 500 {object} schemas.ErrorResponse
// @Router /database/show/{id} [delete]
func (h *Handler) DeleteShow(c echo.Context) error {
	h.Logger.Info("Received request to Delete Show", slog.String("endpoint", "/api/v1/database/show"))

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		h.Logger.Error("Failed parse id", slog.String("error", err.Error()))
		schemas.SendError(c, 400, "Failed to bind request body")
		return err
	}

	if id <= 0 {
		h.Logger.Error("Invalid ID", slog.String("error", "ID must be greater than 0"))
		schemas.SendError(c, 400, "ID must be greater than 0")
		return errors.New("ID must be greater than 0")
	}

	show, getErr := h.ShowRepo.GetById(id)
	if getErr == nil && h.DB != nil {
		epRepo := episodeRepository.NewEpisodeRepository(h.DB)
		epTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(h.DB)
		epContentRepo := epContentRepository.NewEpisodeContentRepository(h.DB)

		episodes, epErr := epRepo.ListByShowID(id)
		if epErr == nil && len(episodes) > 0 {
			epIDs := make([]int64, 0, len(episodes))
			for _, ep := range episodes {
				epIDs = append(epIDs, ep.ID)
			}

			torrents, torrErr := epTorrentRepo.ListByEpisodeIDs(epIDs)
			if torrErr == nil && len(torrents) > 0 {
				var hashes []string
				for _, t := range torrents {
					hashes = append(hashes, t.AllHashes()...)
				}
				if len(hashes) > 0 {
					torrentClient, clientErr := h.newTorrentClient()
					if clientErr != nil {
						h.Logger.Error("Failed to connect to torrent client while deleting show",
							slog.Int64("show_id", id),
							slog.Int("torrent_count", len(hashes)),
							slog.String("error", clientErr.Error()),
						)
						schemas.SendError(c, 500, "Failed to connect to qBittorrent; show was not deleted")
						return clientErr
					}
					if delErr := torrentClient.DeleteTorrents(hashes, true); delErr != nil {
						h.Logger.Error("Failed to delete show torrents from client",
							slog.Int64("show_id", id),
							slog.Int("torrent_count", len(hashes)),
							slog.String("error", delErr.Error()),
						)
						schemas.SendError(c, 500, "Failed to delete show torrents from qBittorrent; show was not deleted")
						return delErr
					}
				}
			}

			if cfg, cfgErr := h.loadConfig(); cfgErr == nil && cfg != nil {
				downloadFolder := ""
				if cfg.QBittorrentDownloadFolder != "" {
					downloadFolder = filepath.Join(cfg.QBittorrentDownloadFolder)
				}
				for _, ep := range episodes {
					if contents, cErr := epContentRepo.ListByEpisodeId(ep.ID); cErr == nil {
						for _, ec := range contents {
							if downloadFolder != "" && ec.Name != "" {
								if _, delErr := utils.DeleteFile(downloadFolder, ec.Name); delErr != nil && !os.IsNotExist(delErr) {
									h.Logger.Error("Failed to delete episode file during show deletion", slog.String("error", delErr.Error()))
								}
							}
							if cfg.ShowsFolder != "" && show.Name != "" {
								_ = utils.DeleteSymlink(cfg.ShowsFolder, show.Name, ep, ec)
							}
						}
					}
					if cfg.ShowsFolder != "" && show.Name != "" {
						_ = utils.DeleteSymlink(cfg.ShowsFolder, show.Name, ep, epContentModel.EpisodeContent{})
						_ = utils.CleanBrokenSymlinksInSeason(cfg.ShowsFolder, show.Name, ep.Season)
					}
				}
			}
		}
	}

	err = h.ShowRepo.DeleteById(id)
	if err != nil {
		if errors.Is(err, repository.ErrShowNotFound) {
			schemas.SendError(c, 404, "Show not found")
			return err
		}
		schemas.SendError(c, 500, "Internal server error")
		return err
	}

	isHtmx := c.Request().Header.Get("HX-Request") == "true"
	if isHtmx {
		c.Response().Header().Set("HX-Redirect", "/")
		return c.NoContent(http.StatusOK)
	}

	schemas.SendSuccess(c, "DeleteShow", model.Show{})
	return nil
}
