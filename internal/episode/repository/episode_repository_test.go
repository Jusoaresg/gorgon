package repository

import (
	"github.com/jusoaresg/gorgon/internal/episode/model"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/testutils"
	"math/rand"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
)

func MakeEpisodeWithDeps(db *sqlx.DB) (model.Episode, error) {
	showRepo := showRepository.NewShowRepository(db)
	seasonRepo := seasonRepository.NewSeasonRepository(db)

	show := testutils.MakeFakeShow()
	showID, err := showRepo.Create(show)
	if err != nil {
		return model.Episode{}, err
	}

	season := testutils.MakeFakeSeason()
	season.ShowID = showID
	seasonID, err := seasonRepo.Create(season)
	if err != nil {
		return model.Episode{}, err
	}

	episode := testutils.MakeFakeEpisode()
	episode.ShowID = showID
	episode.SeasonID = seasonID

	return episode, nil
}

func getEpRepo() (*EpisodeRepository, *sqlx.DB) {
	db := testutils.GetTestDB()
	episodeRepo := NewEpisodeRepository(db)
	return episodeRepo, db
}

func TestEpisodeRepository_Create_Success(t *testing.T) {
	epRepo, db := getEpRepo()

	episode, err := MakeEpisodeWithDeps(db)

	id, err := epRepo.Create(episode)
	assert.NoError(t, err)
	assert.NotZero(t, id)
}

func TestEpisodeRepository_GetByID_Found(t *testing.T) {
	epRepo, db := getEpRepo()

	episode, _ := MakeEpisodeWithDeps(db)
	id, err := epRepo.Create(episode)
	assert.NoError(t, err)

	resultEp, err := epRepo.GetByID(id)
	assert.NoError(t, err)

	episode.ID = id
	assert.EqualValues(t, episode, resultEp)
}

func TestEpisodeRepository_GetByID_NotFound(t *testing.T) {
	epRepo, _ := getEpRepo()

	resultEp, err := epRepo.GetByID(rand.Int63())

	assert.Empty(t, resultEp)
	assert.Error(t, err)
}

func TestEpisodeRepository_List_ReturnAllEpisodes(t *testing.T) {
	epRepo, db := getEpRepo()

	episode1, _ := MakeEpisodeWithDeps(db)
	episode2, _ := MakeEpisodeWithDeps(db)
	episode3, _ := MakeEpisodeWithDeps(db)

	episode1.ID, _ = epRepo.Create(episode1)
	episode2.ID, _ = epRepo.Create(episode2)
	episode3.ID, _ = epRepo.Create(episode3)

	episodes, err := epRepo.List()
	ids := []int64{episodes[0].ID, episodes[1].ID, episodes[2].ID}
	assert.NoError(t, err)
	assert.Contains(t, ids, episode1.ID)
	assert.Contains(t, ids, episode2.ID)
	assert.Contains(t, ids, episode3.ID)
}

func TestEpisodeRepository_List_ReturnEmpty(t *testing.T) {
	episodeRepo, _ := getEpRepo()

	episodes, err := episodeRepo.List()
	assert.NoError(t, err)
	assert.Empty(t, episodes)
}

func TestEpisodeRepository_Update_Success(t *testing.T) {
	episodeRepo, db := getEpRepo()

	episode, _ := MakeEpisodeWithDeps(db)
	id, err := episodeRepo.Create(episode)
	assert.NoError(t, err)

	newEpisode := testutils.MakeFakeEpisode()
	newEpisode.ID = id
	newEpisode.ShowID = episode.ShowID
	newEpisode.SeasonID = episode.SeasonID

	err = episodeRepo.Update(newEpisode)
	assert.NoError(t, err)

	resultEpisode, err := episodeRepo.GetByID(newEpisode.ID)
	assert.EqualValues(t, newEpisode, resultEpisode)
}

func TestEpisodeRepository_Update_NotFound(t *testing.T) {
	episodeRepo, db := getEpRepo()

	episode, _ := MakeEpisodeWithDeps(db)

	err := episodeRepo.Update(episode)
	assert.Error(t, err)

	resultEpisode, err := episodeRepo.GetByID(episode.ID)
	assert.NotEqualValues(t, episode, resultEpisode)
}

func TestEpisodeRepository_DeleteByID_Success(t *testing.T) {
	episodeRepo, db := getEpRepo()

	episode, _ := MakeEpisodeWithDeps(db)
	id, err := episodeRepo.Create(episode)
	assert.NoError(t, err)

	err = episodeRepo.DeleteByID(id)
	assert.NoError(t, err)

	_, err = episodeRepo.GetByID(id)
	assert.Error(t, err)
}

func TestEpisodeRepository_DeleteTx_Commit(t *testing.T) {
	episodeRepo, db := getEpRepo()

	episode, _ := MakeEpisodeWithDeps(db)
	id, err := episodeRepo.Create(episode)
	assert.NoError(t, err)

	tx, err := db.Beginx()
	assert.NoError(t, err)

	err = episodeRepo.DeleteTx(tx, id)
	assert.NoError(t, err)

	err = tx.Commit()
	assert.NoError(t, err)

	_, err = episodeRepo.GetByID(id)
	assert.Error(t, err)
}

func TestEpisodeRepository_DeleteTx_Rollback(t *testing.T) {
	episodeRepo, db := getEpRepo()

	episode, _ := MakeEpisodeWithDeps(db)
	id, err := episodeRepo.Create(episode)
	assert.NoError(t, err)

	tx, err := db.Beginx()
	assert.NoError(t, err)

	err = episodeRepo.DeleteTx(tx, id)
	assert.NoError(t, err)

	err = tx.Rollback()
	assert.NoError(t, err)

	found, err := episodeRepo.GetByID(id)
	assert.NoError(t, err)
	assert.Equal(t, id, found.ID)
}

func TestEpisodeRepository_CreateTx_And_UpdateTx(t *testing.T) {
	episodeRepo, db := getEpRepo()

	episode, _ := MakeEpisodeWithDeps(db)

	tx, err := db.Beginx()
	assert.NoError(t, err)

	id, err := episodeRepo.CreateTx(tx, episode)
	assert.NoError(t, err)
	assert.NotZero(t, id)

	episode.ID = id
	episode.Name = "Transaction Updated Name"
	err = episodeRepo.UpdateTx(tx, episode)
	assert.NoError(t, err)

	err = tx.Commit()
	assert.NoError(t, err)

	found, err := episodeRepo.GetByID(id)
	assert.NoError(t, err)
	assert.Equal(t, "Transaction Updated Name", found.Name)
}

func TestEpisodeRepository_ListByFilters(t *testing.T) {
	episodeRepo, db := getEpRepo()

	episode, _ := MakeEpisodeWithDeps(db)
	episode.Tracking = model.TrackingWanted
	id, err := episodeRepo.Create(episode)
	assert.NoError(t, err)

	byShow, err := episodeRepo.ListByShowID(episode.ShowID)
	assert.NoError(t, err)
	assert.NotEmpty(t, byShow)

	bySeason, err := episodeRepo.ListBySeasonID(int(episode.SeasonID))
	assert.NoError(t, err)
	assert.NotEmpty(t, bySeason)

	byTracking, err := episodeRepo.ListByTracking(model.TrackingWanted)
	assert.NoError(t, err)
	assert.NotEmpty(t, byTracking)

	allByID, err := episodeRepo.GetAllByID(id)
	assert.NoError(t, err)
	assert.Len(t, allByID, 1)
}
