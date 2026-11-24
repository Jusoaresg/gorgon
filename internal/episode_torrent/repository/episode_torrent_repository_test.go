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

func TestEpisodeTorrentRepository_PreviousSnapshot_RestoreAndClear(t *testing.T) {
	db, repo := getEpTorrentRepo()

	oldTorrent := MakeEpisodeTorrentWithDeps(db)
	_, err := repo.Upsert(oldTorrent)
	assert.NoError(t, err)

	newTorrent := model.EpisodeTorrent{
		EpisodeId:   oldTorrent.EpisodeId,
		Hash:        "newhash999",
		Title:       "Show.S01E01.2160p.WEB-DL",
		Indexer:     "NewIndexer",
		InfoUrl:     "https://indexer.info/release/2",
		PublishDate: "2026-08-02T00:00:00Z",
		Score:       90,
		CreatedAt:   1754001000,
		IsManual:    true,
	}
	newTorrent.SnapshotPreviousFrom(oldTorrent)

	_, err = repo.Upsert(newTorrent)
	assert.NoError(t, err)

	saved, err := repo.GetByEpisodeID(oldTorrent.EpisodeId)
	assert.NoError(t, err)
	assert.True(t, saved.IsManual)
	assert.True(t, saved.HasPrevious())
	assert.Equal(t, oldTorrent.Hash, saved.PreviousHash)
	assert.Equal(t, oldTorrent.Title, saved.PreviousTitle)
	assert.Equal(t, oldTorrent.Indexer, saved.PreviousIndexer)
	assert.Equal(t, oldTorrent.InfoUrl, saved.PreviousInfoUrl)
	assert.Equal(t, oldTorrent.PublishDate, saved.PreviousPublishDate)
	assert.Equal(t, oldTorrent.Score, saved.PreviousScore)

	// RestorePrevious should revert back to oldTorrent and clear previous_* & is_manual
	restored, err := repo.RestorePrevious(oldTorrent.EpisodeId)
	assert.NoError(t, err)
	assert.Equal(t, oldTorrent.Hash, restored.Hash)
	assert.Equal(t, oldTorrent.Title, restored.Title)
	assert.Equal(t, oldTorrent.Indexer, restored.Indexer)
	assert.Equal(t, oldTorrent.InfoUrl, restored.InfoUrl)
	assert.Equal(t, oldTorrent.PublishDate, restored.PublishDate)
	assert.Equal(t, oldTorrent.Score, restored.Score)
	assert.False(t, restored.IsManual)
	assert.False(t, restored.HasPrevious())

	// Re-apply snapshot and test ClearPrevious
	_, err = repo.Upsert(newTorrent)
	assert.NoError(t, err)
	err = repo.ClearPrevious(oldTorrent.EpisodeId)
	assert.NoError(t, err)

	cleared, err := repo.GetByEpisodeID(oldTorrent.EpisodeId)
	assert.NoError(t, err)
	assert.Equal(t, newTorrent.Hash, cleared.Hash)
	assert.False(t, cleared.HasPrevious())
	assert.Empty(t, cleared.PreviousTitle)
	assert.Zero(t, cleared.PreviousScore)
}

func TestEpisodeTorrentRepository_Upsert_PreservesPreviousHashAndClear(t *testing.T) {
	db, repo := getEpTorrentRepo()

	torrent := MakeEpisodeTorrentWithDeps(db)
	originalHash := torrent.Hash
	_, err := repo.Upsert(torrent)
	assert.NoError(t, err)

	// Upserting with a new hash (upgrade) must preserve originalHash in PreviousHash
	torrent.Hash = "newhash12345"
	_, err = repo.Upsert(torrent)
	assert.NoError(t, err)

	updated, err := repo.GetByEpisodeID(torrent.EpisodeId)
	assert.NoError(t, err)
	assert.Equal(t, "newhash12345", updated.Hash)
	assert.Equal(t, originalHash, updated.PreviousHash)
	assert.Equal(t, []string{"newhash12345", originalHash}, updated.AllHashes())

	// Upserting with the same hash (case-insensitive) must NOT overwrite PreviousHash
	torrent.Hash = "NEWHASH12345"
	_, err = repo.Upsert(torrent)
	assert.NoError(t, err)

	sameHashResult, err := repo.GetByEpisodeID(torrent.EpisodeId)
	assert.NoError(t, err)
	assert.Equal(t, originalHash, sameHashResult.PreviousHash)

	// ClearPreviousHash must reset PreviousHash to empty
	err = repo.ClearPreviousHash(torrent.EpisodeId)
	assert.NoError(t, err)

	cleared, err := repo.GetByEpisodeID(torrent.EpisodeId)
	assert.NoError(t, err)
	assert.Equal(t, "", cleared.PreviousHash)
	assert.Equal(t, []string{"NEWHASH12345"}, cleared.AllHashes())
}
