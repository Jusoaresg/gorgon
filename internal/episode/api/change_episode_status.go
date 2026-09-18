package api

import (
	"log/slog"

	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/qbittorrent/service"
	episodeEvents "github.com/jusoaresg/gorgon/internal/episode/events"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	"github.com/jusoaresg/gorgon/internal/episode/schema"
	epContentModel "github.com/jusoaresg/gorgon/internal/episode_content/model"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/jusoaresg/gorgon/utils"

	"github.com/labstack/echo/v4"
)

// @BasePath /api/v1

// @Summary Change Episodes Status
// @Description Change Episodes Tracking Status
// @Tags Database/Show/Episode
// @Produce json
// @Param request body schema.ChangeEpisodeTrackingRequest true "Request Body"
// @Success 200 {object} schemas.DefaultResponse
// @Failure 400 {object} schemas.ErrorResponse
// @Failure 500 {object} schemas.ErrorResponse
// @Router /database/show/episode/status [post]
func (h *Handler) ChangeEpisodeStatus(c echo.Context) error {
	h.Logger.Info("Received request to Change Episode Status", slog.String("endpoint", "/database/show/episode/status"), slog.String("method", "post"))

	var request schema.ChangeEpisodeTrackingRequest

	if err := c.Bind(&request); err != nil {
		schemas.SendError(c, 500, "Failed to bind body")
		return err
	}

	qbittorrentService, err := service.NewQBittorrentService(h.Logger)
	if err != nil {
		h.Logger.Error("Failed to create qbittorrent service", slog.String("error", err.Error()))
	}

	cfg, _ := config.LoadConfig()

	episodes, err := h.EpisodeRepo.GetAllByID(utils.ToInt64Slice(request.EpisodeIds)...)
	if err != nil {
		h.Logger.Error("Error while fetching episode from database", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Error while fetching episode")
		return err
	}

	for _, episode := range episodes {
		if request.Tracking != episodeModel.TrackingDownloaded {
			show, err := h.ShowRepo.GetById(episode.ShowID)
			if err == nil && cfg != nil {
				episodeContents, err := h.EpisodeContentRepo.ListByEpisodeId(episode.ID)
				if err == nil {
					for _, ec := range episodeContents {
						_ = utils.DeleteSymlink(cfg.ShowsFolder, show.Name, episode, ec)
						_ = h.EpisodeContentRepo.DeleteById(ec.ID)
					}
				}
				_ = utils.DeleteSymlink(cfg.ShowsFolder, show.Name, episode, epContentModel.EpisodeContent{})
				_ = utils.CleanBrokenSymlinksInSeason(cfg.ShowsFolder, show.Name, episode.Season)
			}
		}

		episodeTorrent, err := h.EpisodeTorrentRepo.GetByEpisodeID(episode.ID)
		if err == nil && episodeTorrent.Hash != "" {
			if qbittorrentService != nil {
				err := qbittorrentService.DeleteTorrent(episodeTorrent.Hash, true)
				if err != nil {
					h.Logger.Error(
						"Error while deleting torrent",
						slog.String("error", err.Error()),
						slog.Int64("episode_id", episode.ID),
					)
				}
			}
			if err := h.EpisodeTorrentRepo.DeleteByEpisodeID(episode.ID); err != nil {
				h.Logger.Error(
					"Error while deleting episode torrent",
					slog.String("error", err.Error()),
					slog.Int64("episode_id", episode.ID),
				)
			}
		}
		episode.Tracking = request.Tracking

		if err := h.EpisodeRepo.Update(episode); err != nil {
			schemas.SendError(c, 500, "Failed to update episode tracking status")
			return err
		}

		episodeEvents.EmitEpisodeTrackingUpdatedEvent(episode.ID, episode.Tracking, "")
	}

	h.Logger.Info("Successfully updated episodes", slog.Any("Episodes", episodes))
	schemas.SendSuccess(c, "Change episodes tracking", map[string]any{
		"toastMessage": "Tracking updated",
	})
	return nil
}
