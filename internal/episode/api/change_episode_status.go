package api

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	qbittorrentService "github.com/jusoaresg/gorgon/external/qbittorrent/service"
	episodeEvents "github.com/jusoaresg/gorgon/internal/episode/events"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	"github.com/jusoaresg/gorgon/internal/episode/schema"
	epContentModel "github.com/jusoaresg/gorgon/internal/episode_content/model"
	episodeTorrentModel "github.com/jusoaresg/gorgon/internal/episode_torrent/model"
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

	cfg, _ := h.loadConfig()

	episodeIDs := utils.ToInt64Slice(request.EpisodeIds)
	episodes, err := h.EpisodeRepo.GetAllByID(episodeIDs...)
	if err != nil {
		h.Logger.Error("Error while fetching episode from database", slog.String("error", err.Error()))
		schemas.SendError(c, 500, "Error while fetching episode")
		return err
	}

	torrentsByEpisodeID := make(map[int64]episodeTorrentModel.EpisodeTorrent, len(episodes))
	if torrents, err := h.EpisodeTorrentRepo.ListByEpisodeIDs(episodeIDs); err == nil {
		for _, t := range torrents {
			torrentsByEpisodeID[t.EpisodeId] = t
		}
	}

	var (
		torrentClient     qbittorrentService.TorrentDeleter
		torrentClientInit bool
		torrentClientErr  error
	)
	getTorrentClient := func() (qbittorrentService.TorrentDeleter, error) {
		if !torrentClientInit {
			torrentClientInit = true
			torrentClient, torrentClientErr = h.newTorrentClient()
			if torrentClientErr != nil {
				h.Logger.Error("Failed to create qbittorrent service", slog.String("error", torrentClientErr.Error()))
			}
		}
		return torrentClient, torrentClientErr
	}

	downloadFolder := ""
	if cfg != nil && cfg.QBittorrentDownloadFolder != "" {
		downloadFolder = filepath.Join(cfg.QBittorrentDownloadFolder)
	}

	updatedCount := 0
	failedCount := 0

	for _, episode := range episodes {
		episodeTorrent, hasTorrent := torrentsByEpisodeID[episode.ID]
		hashes := episodeTorrent.AllHashes()

		if len(hashes) > 0 {
			qbit, qbitErr := getTorrentClient()
			if qbitErr != nil || qbit == nil {
				h.Logger.Error(
					"Skipping episode status update because qBittorrent client is unavailable",
					slog.Int64("episode_id", episode.ID),
					slog.Any("hashes", hashes),
				)
				failedCount++
				continue
			}

			if err := qbit.DeleteTorrents(hashes, true); err != nil {
				h.Logger.Error(
					"Error while deleting torrent; keeping episode and torrent record intact",
					slog.String("error", err.Error()),
					slog.Int64("episode_id", episode.ID),
					slog.Any("hashes", hashes),
				)
				failedCount++
				continue
			}
		}

		if request.Tracking != episodeModel.TrackingDownloaded {
			show, showErr := h.ShowRepo.GetById(episode.ShowID)
			episodeContents, contentErr := h.EpisodeContentRepo.ListByEpisodeId(episode.ID)
			if contentErr == nil {
				for _, ec := range episodeContents {
					if downloadFolder != "" && ec.Name != "" {
						if _, delErr := utils.DeleteFile(downloadFolder, ec.Name); delErr != nil && !os.IsNotExist(delErr) {
							h.Logger.Error("Failed to delete downloaded episode file",
								slog.Int64("episode_id", episode.ID),
								slog.String("file", ec.Name),
								slog.String("error", delErr.Error()),
							)
						}
					}
					if showErr == nil && cfg != nil {
						_ = utils.DeleteSymlink(cfg.ShowsFolder, show.Name, episode, ec)
					}
					_ = h.EpisodeContentRepo.DeleteById(ec.ID)
				}
			}
			if showErr == nil && cfg != nil {
				_ = utils.DeleteSymlink(cfg.ShowsFolder, show.Name, episode, epContentModel.EpisodeContent{})
				_ = utils.CleanBrokenSymlinksInSeason(cfg.ShowsFolder, show.Name, episode.Season)
			}
		}

		if hasTorrent {
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
			h.Logger.Error(
				"Failed to update episode tracking status",
				slog.Int64("episode_id", episode.ID),
				slog.String("error", err.Error()),
			)
			failedCount++
			continue
		}

		updatedCount++
		episodeEvents.EmitEpisodeTrackingUpdatedEvent(episode.ID, episode.Tracking, "")
	}

	if failedCount > 0 {
		msg := fmt.Sprintf("Failed to update %d episode(s); check qBittorrent connection", failedCount)
		if updatedCount > 0 {
			msg = fmt.Sprintf("Updated %d episode(s), but %d failed (kept intact — check qBittorrent)", updatedCount, failedCount)
		}
		h.Logger.Warn("Episode status update completed with failures",
			slog.Int("updated", updatedCount),
			slog.Int("failed", failedCount),
		)
		schemas.SendError(c, 500, msg)
		return nil
	}

	h.Logger.Info("Successfully updated episodes", slog.Int("updated", updatedCount))
	schemas.SendSuccess(c, "Change episodes tracking", map[string]any{
		"toastMessage": "Tracking updated",
	})
	return nil
}
