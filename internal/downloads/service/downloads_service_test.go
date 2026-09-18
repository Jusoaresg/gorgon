package service

import (
	"log/slog"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	episodeTorrentModel "github.com/jusoaresg/gorgon/internal/episode_torrent/model"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestService(db *sqlx.DB) *DownloadsService {
	return NewDownloadsService(db, slog.Default())
}

func seedShowAndEpisode(db *sqlx.DB, t *testing.T, hash string, tracking string) {
	t.Helper()

	showRepo := showRepository.NewShowRepository(db)
	seasonRepo := seasonRepository.NewSeasonRepository(db)
	episodeRepo := episodeRepository.NewEpisodeRepository(db)
	episodeTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(db)

	show := testutils.MakeFakeShow()
	show.Name = "Breaking Test"
	showID, err := showRepo.Create(show)
	require.NoError(t, err)

	season := testutils.MakeFakeSeason()
	season.ShowID = showID
	seasonID, err := seasonRepo.Create(season)
	require.NoError(t, err)

	episode := testutils.MakeFakeEpisode()
	episode.ShowID = showID
	episode.SeasonID = seasonID
	episode.Name = "Pilot"
	episode.Season = 1
	episode.Number = 1
	episode.Tracking = tracking
	episodeID, err := episodeRepo.Create(episode)
	require.NoError(t, err)

	_, err = episodeTorrentRepo.Upsert(episodeTorrentModel.EpisodeTorrent{
		EpisodeId: episodeID,
		Hash:      hash,
	})
	require.NoError(t, err)
}

func gorgonTorrent(hash, state string) schema.CheckTorrentResponse {
	return schema.CheckTorrentResponse{
		Name:     "Gorgon Torrent",
		Hash:     hash,
		State:    state,
		Category: gorgonCategory,
		Progress: 0.5,
	}
}

func TestBuildDownloads_OnlyGorgonAndActive(t *testing.T) {
	db := testutils.GetTestDB()
	svc := newTestService(db)

	seedShowAndEpisode(db, t, "abc123", episodeModel.TrackingSnatched)

	torrents := []schema.CheckTorrentResponse{
		gorgonTorrent("abc123", "downloading"),
		gorgonTorrent("deadbeef", "stalledDL"),
		{Name: "Other", Hash: "otherhash", State: "downloading", Category: "movies"},
		gorgonTorrent("finished", "uploading"),
		gorgonTorrent("seeding", "stalledUP"),
		gorgonTorrent("broken", "error"),
	}

	items, err := svc.BuildDownloads(torrents)
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "abc123", items[0].Torrent.Hash)
	require.NotNil(t, items[0].Episode)
	assert.Equal(t, "Breaking Test", items[0].Episode.ShowName)
	assert.Equal(t, "Pilot", items[0].Episode.Name)
	assert.Equal(t, 1, items[0].Episode.Season)
	assert.Equal(t, 1, items[0].Episode.Number)

	assert.Equal(t, "deadbeef", items[1].Torrent.Hash)
	assert.Nil(t, items[1].Episode)
}

func TestBuildDownloads_MatchHashCaseInsensitive(t *testing.T) {
	db := testutils.GetTestDB()
	svc := newTestService(db)

	seedShowAndEpisode(db, t, "ABCDEF123456", episodeModel.TrackingSnatched)

	items, err := svc.BuildDownloads([]schema.CheckTorrentResponse{
		gorgonTorrent("abcdef123456", "downloading"),
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NotNil(t, items[0].Episode)
	assert.Equal(t, "Breaking Test", items[0].Episode.ShowName)
}

func TestBuildDownloads_NoEpisodesInDatabase(t *testing.T) {
	db := testutils.GetTestDB()
	svc := newTestService(db)

	items, err := svc.BuildDownloads([]schema.CheckTorrentResponse{
		gorgonTorrent("abc123", "downloading"),
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Nil(t, items[0].Episode)
}

func TestBuildDownloads_NoActiveTorrents(t *testing.T) {
	db := testutils.GetTestDB()
	svc := newTestService(db)

	items, err := svc.BuildDownloads(nil)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestBuildDownloads_PendingImportKept(t *testing.T) {
	db := testutils.GetTestDB()
	svc := newTestService(db)

	seedShowAndEpisode(db, t, "abc123", episodeModel.TrackingSnatched)

	items, err := svc.BuildDownloads([]schema.CheckTorrentResponse{
		{Name: "Done Torrent", Hash: "abc123", State: "uploading", Category: gorgonCategory, Progress: 1.0},
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NotNil(t, items[0].Episode)
	assert.Equal(t, float32(1.0), items[0].Torrent.Progress)
	assert.Equal(t, "uploading", items[0].Torrent.State)
	assert.Equal(t, episodeModel.TrackingSnatched, items[0].Episode.Tracking)
}

func TestBuildDownloads_DownloadedEpisodeRemoved(t *testing.T) {
	db := testutils.GetTestDB()
	svc := newTestService(db)

	seedShowAndEpisode(db, t, "abc123", episodeModel.TrackingDownloaded)

	items, err := svc.BuildDownloads([]schema.CheckTorrentResponse{
		{Name: "Done Torrent", Hash: "abc123", State: "stalledUP", Category: gorgonCategory, Progress: 1.0},
	})
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestBuildDownloads_FinishedWithoutEpisodeRemoved(t *testing.T) {
	db := testutils.GetTestDB()
	svc := newTestService(db)

	items, err := svc.BuildDownloads([]schema.CheckTorrentResponse{
		gorgonTorrent("orphan", "uploading"),
	})
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestBuildDownloads_SortActiveFirstThenAddedOn(t *testing.T) {
	db := testutils.GetTestDB()
	svc := newTestService(db)

	seedShowAndEpisode(db, t, "pending1", episodeModel.TrackingSnatched)
	seedShowAndEpisode(db, t, "pending2", episodeModel.TrackingSnatched)

	torrents := []schema.CheckTorrentResponse{
		{Name: "Pending2", Hash: "pending2", State: "uploading", Category: gorgonCategory, AddedOn: 400},
		{Name: "B", Hash: "bbbb", State: "downloading", Category: gorgonCategory, AddedOn: 200},
		{Name: "A", Hash: "aaaa", State: "downloading", Category: gorgonCategory, AddedOn: 100},
		{Name: "Pending1", Hash: "pending1", State: "stalledUP", Category: gorgonCategory, AddedOn: 300},
	}

	items, err := svc.BuildDownloads(torrents)
	require.NoError(t, err)
	require.Len(t, items, 4)

	assert.Equal(t, "aaaa", items[0].Torrent.Hash, "active, added first")
	assert.Equal(t, "bbbb", items[1].Torrent.Hash, "active, added second")
	assert.Equal(t, "pending1", items[2].Torrent.Hash, "waiting to import, added first")
	assert.Equal(t, "pending2", items[3].Torrent.Hash, "waiting to import, added second")
}

func TestComputeSummary(t *testing.T) {
	db := testutils.GetTestDB()
	svc := newTestService(db)

	items := []DownloadItem{
		{
			Torrent: schema.CheckTorrentResponse{
				DlSpeed: 5000000,
				UpSpeed: 1000000,
				State:   "downloading",
			},
		},
		{
			Torrent: schema.CheckTorrentResponse{
				DlSpeed: 2000000,
				UpSpeed: 500000,
				State:   "stalledDL",
			},
		},
		{
			Torrent: schema.CheckTorrentResponse{
				DlSpeed: 0,
				UpSpeed: 200000,
				State:   "uploading",
			},
		},
	}

	summary := svc.ComputeSummary(items)
	assert.Equal(t, 7000000, summary.TotalDlSpeed)
	assert.Equal(t, 1700000, summary.TotalUpSpeed)
	assert.Equal(t, 2, summary.ActiveCount)
}
