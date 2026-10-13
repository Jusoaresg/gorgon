package repository

import (
	"testing"

	"github.com/jusoaresg/gorgon/internal/episode_torrent/model"
	"github.com/stretchr/testify/assert"

	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/testutils"

	"github.com/jmoiron/sqlx"
)

func getEpTorrentRepo() (*sqlx.DB, *EpisodeTorrentRepository) {
	db := testutils.GetTestDB()
	repo := NewEpisodeTorrentRepository(db)
	return db, repo
}

func MakeEpisodeTorrentWithDeps(db *sqlx.DB) model.EpisodeTorrent {
	fakeShow := testutils.MakeFakeShow()

	showRepo := showRepository.NewShowRepository(db)
	showID, _ := showRepo.Create(fakeShow)

	fakeSeason := testutils.MakeFakeSeason()
	fakeSeason.ShowID = showID

	seasonRepo := seasonRepository.NewSeasonRepository(db)
	seasonID, _ := seasonRepo.Create(fakeSeason)

	fakeEpisode := testutils.MakeFakeEpisode()
	fakeEpisode.ShowID = showID
	fakeEpisode.SeasonID = seasonID

	episodeRepo := episodeRepository.NewEpisodeRepository(db)
	episodeID, _ := episodeRepo.Create(fakeEpisode)

	return model.EpisodeTorrent{
		EpisodeId:   episodeID,
		Hash:        "abcdef1234567890",
		Title:       "Show.S01E01.1080p.WEB-DL",
		Indexer:     "TestIndexer",
		InfoUrl:     "https://indexer.info/release/1",
		PublishDate: "2026-08-01T00:00:00Z",
		Score:       50,
		CreatedAt:   1754000000,
	}
}

func TestEpisodeTorrentRepository_Upsert_Create(t *testing.T) {
	db, repo := getEpTorrentRepo()

	torrent := MakeEpisodeTorrentWithDeps(db)
	id, err := repo.Upsert(torrent)
	assert.NoError(t, err)
	assert.NotZero(t, id)

	result, err := repo.GetByEpisodeID(torrent.EpisodeId)
	assert.NoError(t, err)
	assert.Equal(t, torrent.Hash, result.Hash)
	assert.Equal(t, torrent.EpisodeId, result.EpisodeId)
}

func TestEpisodeTorrentRepository_Upsert_Update(t *testing.T) {
	db, repo := getEpTorrentRepo()

	torrent := MakeEpisodeTorrentWithDeps(db)
	_, err := repo.Upsert(torrent)
	assert.NoError(t, err)

	torrent.Hash = "newhash1234"
	torrent.Title = "Show.S01E01.720p.WEB-DL"
	torrent.Score = 75
	_, err = repo.Upsert(torrent)
	assert.NoError(t, err)

	result, err := repo.GetByEpisodeID(torrent.EpisodeId)
	assert.NoError(t, err)
	assert.Equal(t, "newhash1234", result.Hash)
	assert.Equal(t, "Show.S01E01.720p.WEB-DL", result.Title)
	assert.Equal(t, 75, result.Score)
}

func TestEpisodeTorrentRepository_GetByEpisodeID_NotFound(t *testing.T) {
	_, repo := getEpTorrentRepo()

	result, err := repo.GetByEpisodeID(999999)
	assert.Empty(t, result)
	assert.Error(t, err)
}

func TestEpisodeTorrentRepository_GetByHash_CaseInsensitive(t *testing.T) {
	db, repo := getEpTorrentRepo()

	torrent := MakeEpisodeTorrentWithDeps(db)
	_, err := repo.Upsert(torrent)
	assert.NoError(t, err)

	result, err := repo.GetByHash("ABCDEF1234567890")
	assert.NoError(t, err)
	assert.Equal(t, torrent.EpisodeId, result.EpisodeId)
}

func TestEpisodeTorrentRepository_DeleteByEpisodeID(t *testing.T) {
	db, repo := getEpTorrentRepo()

	torrent := MakeEpisodeTorrentWithDeps(db)
	_, err := repo.Upsert(torrent)
	assert.NoError(t, err)

	err = repo.DeleteByEpisodeID(torrent.EpisodeId)
	assert.NoError(t, err)

	result, err := repo.GetByEpisodeID(torrent.EpisodeId)
	assert.Empty(t, result)
	assert.Error(t, err)
}

func TestEpisodeTorrentRepository_DeleteByEpisodeIDs(t *testing.T) {
	db, repo := getEpTorrentRepo()

	t1 := MakeEpisodeTorrentWithDeps(db)
	t2 := MakeEpisodeTorrentWithDeps(db)
	_, err := repo.Upsert(t1)
	assert.NoError(t, err)
	_, err = repo.Upsert(t2)
	assert.NoError(t, err)

	err = repo.DeleteByEpisodeIDs(t1.EpisodeId, t2.EpisodeId)
	assert.NoError(t, err)

	_, err = repo.GetByEpisodeID(t1.EpisodeId)
	assert.Error(t, err)
	_, err = repo.GetByEpisodeID(t2.EpisodeId)
	assert.Error(t, err)
}

func TestEpisodeTorrentRepository_UpdateStallState(t *testing.T) {
	db, repo := getEpTorrentRepo()

	torrent := MakeEpisodeTorrentWithDeps(db)
	_, err := repo.Upsert(torrent)
	assert.NoError(t, err)

	err = repo.UpdateStallState(torrent.EpisodeId, 1754000500, 52428800)
	assert.NoError(t, err)

	result, err := repo.GetByEpisodeID(torrent.EpisodeId)
	assert.NoError(t, err)
	assert.Equal(t, int64(1754000500), result.StalledSince)
	assert.Equal(t, int64(52428800), result.LastAmountLeft)

	// Re-upserting a new torrent for the episode must reset stall state
	torrent.Hash = "freshhash999"
	_, err = repo.Upsert(torrent)
	assert.NoError(t, err)

	resetResult, err := repo.GetByEpisodeID(torrent.EpisodeId)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), resetResult.StalledSince)
	assert.Equal(t, int64(-1), resetResult.LastAmountLeft)
}
