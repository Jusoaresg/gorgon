package api

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/jusoaresg/gorgon/internal/episode/events"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	epContentModel "github.com/jusoaresg/gorgon/internal/episode_content/model"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/jusoaresg/gorgon/utils"

	"github.com/labstack/echo/v4"
)

// @BasePath /api/v1

// @Summary Delete Downloaded Episode
// @Description Delete Downloaded Episode
// @Tags Database/Episodes
// @Produce json
// @Param id path int true "Episode ID"
// @Success 200 {object} schemas.DefaultResponse
// @Failure 400 {object} schemas.ErrorResponse
// @Failure 500 {object} schemas.ErrorResponse
// @Router /database/show/episode/{id} [delete]
func (h *Handler) DeleteDownloadedEpisode(c echo.Context) error {
	h.Logger.Info("Received request to Delete Downloaded Episode", slog.String("endpoint", "/database/show/episode/:id"), slog.String("method", "delete"))

	cfg, err := h.loadConfig()
	if err != nil {
		schemas.SendError(c, 400, "Error while loading config file")
		return err
	}

	id := c.Param("id")
	idInt64, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		schemas.SendError(c, 400, "Error while converting id to int")
		return err
	}

	ep, err := h.EpisodeRepo.GetByID(idInt64)
	if err != nil {
		h.Logger.Error("Error while fetching episode from database to delete", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Error while fetching episode to delete")
		return err
	}

	episodeContents, err := h.EpisodeContentRepo.ListByEpisodeId(idInt64)
	if err != nil {
		h.Logger.Error("Error while fetching episode content from database", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Error while fetching episode content")
		return err
	}

	show, err := h.ShowRepo.GetById(ep.ShowID)
	if err != nil {
		h.Logger.Error("Error while fetching show from database", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Error while fetching show")
		return err
	}

	episodeTorrent, _ := h.EpisodeTorrentRepo.GetByEpisodeID(ep.ID)
	hashes := episodeTorrent.AllHashes()
	if len(hashes) > 0 {
		torrentService, err := h.newTorrentClient()
		if err != nil {
			h.Logger.Error("Failed to initialize torrent client for episode deletion",
				slog.Int64("episode_id", ep.ID),
				slog.Any("hashes", hashes),
				slog.String("error", err.Error()),
			)
			schemas.SendError(c, 500, "Failed to connect to qBittorrent; episode was not deleted")
			return err
		}

		if err := torrentService.DeleteTorrents(hashes, true); err != nil {
			h.Logger.Error("Failed to delete torrent from client",
				slog.Int64("episode_id", ep.ID),
				slog.Any("hashes", hashes),
				slog.String("error", err.Error()),
			)
			schemas.SendError(c, 500, "Failed to delete torrent from qBittorrent; episode was not deleted")
			return err
		}
	}

	downloadFolder := filepath.Join(cfg.QBittorrentDownloadFolder)

	for _, episodeContent := range episodeContents {
		if downloadFolder != "" && episodeContent.Name != "" {
			if _, err := utils.DeleteFile(downloadFolder, episodeContent.Name); err != nil && !os.IsNotExist(err) {
				h.Logger.Error("Failed to delete downloaded episode", slog.String("error", err.Error()))
			}
		}

		if err := utils.DeleteSymlink(cfg.ShowsFolder, show.Name, ep, episodeContent); err != nil {
			h.Logger.Error("Failed to delete episode symlink from shows folder", slog.String("error", err.Error()))
		}

		if err := h.EpisodeContentRepo.DeleteById(episodeContent.ID); err != nil {
			h.Logger.Error("Failed to delete episode content from the database (The File was deleted)", slog.String("error", err.Error()))
		}
	}

	_ = utils.DeleteSymlink(cfg.ShowsFolder, show.Name, ep, epContentModel.EpisodeContent{})
	_ = utils.CleanBrokenSymlinksInSeason(cfg.ShowsFolder, show.Name, ep.Season)

	if err := h.EpisodeTorrentRepo.DeleteByEpisodeID(ep.ID); err != nil {
		h.Logger.Error("Failed to delete episode torrent association", slog.Int64("episode_id", ep.ID), slog.String("error", err.Error()))
	}

	ep.SetNotInstalled()
	err = h.EpisodeRepo.Update(ep)
	if err != nil {
		h.Logger.Error("Failed to set episode to not installed", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Failed to update episode status")
		return err
	}

	episode.EmitEpisodeTrackingUpdatedEvent(ep.ID, episodeModel.TrackingSkipped, "")

	h.Logger.Info("Successfully deleted downloaded episode", slog.Int64("episode_id", ep.ID))
	schemas.SendSuccess(c, "Delete Downloaded Episode", map[string]any{
		"toastMessage": "Episode deleted",
	})
	return nil
}
