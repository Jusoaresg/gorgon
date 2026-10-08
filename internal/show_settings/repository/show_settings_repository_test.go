package repository

import (
	"testing"

	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/internal/show_settings/model"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/assert"
)

func getShowSettingsRepo() ShowSettingsRepository {
	return NewShowSettingsRepository(testutils.GetTestDB())
}

func createTestShow(t *testing.T, repo ShowSettingsRepository) int64 {
	t.Helper()
	showRepo := showRepository.NewShowRepository(repo.db)
	id, err := showRepo.Create(testutils.MakeFakeShow())
	assert.NoError(t, err)
	return id
}

func TestShowSettingsRepository_UpsertAndGet(t *testing.T) {
	repo := getShowSettingsRepo()
	showID := createTestShow(t, repo)

	err := repo.Upsert(model.ShowSettings{
		ShowID:     showID,
		UseAliases: true,
		OnlyLatin:  false,
	})
	assert.NoError(t, err)

	got, err := repo.GetByShowID(showID)
	assert.NoError(t, err)
	assert.Equal(t, showID, got.ShowID)
	assert.True(t, got.UseAliases)
	assert.False(t, got.OnlyLatin)
}

func TestShowSettingsRepository_UpsertUpdatesExisting(t *testing.T) {
	repo := getShowSettingsRepo()
	showID := createTestShow(t, repo)

	assert.NoError(t, repo.Upsert(model.ShowSettings{ShowID: showID, UseAliases: true, OnlyLatin: true}))
	assert.NoError(t, repo.Upsert(model.ShowSettings{ShowID: showID, UseAliases: false, OnlyLatin: true}))

	got, err := repo.GetByShowID(showID)
	assert.NoError(t, err)
	assert.False(t, got.UseAliases)

	var count int
	assert.NoError(t, repo.db.Get(&count, "SELECT COUNT(*) FROM show_settings WHERE show_id = ?", showID))
	assert.Equal(t, 1, count)
}

func TestShowSettingsRepository_GetByShowIDNotFound(t *testing.T) {
	repo := getShowSettingsRepo()

	_, err := repo.GetByShowID(9999)
	assert.Error(t, err)
}

func TestShowSettingsRepository_DeleteByShowID(t *testing.T) {
	repo := getShowSettingsRepo()
	showID := createTestShow(t, repo)

	assert.NoError(t, repo.Upsert(model.ShowSettings{ShowID: showID}))
	assert.NoError(t, repo.DeleteByShowID(showID))

	_, err := repo.GetByShowID(showID)
	assert.Error(t, err)
}

func TestShowSettingsRepository_DeleteByShowIDNotFound(t *testing.T) {
	repo := getShowSettingsRepo()

	err := repo.DeleteByShowID(9999)
	assert.ErrorIs(t, err, ErrShowSettingsNotFound)
}

func TestShowSettingsRepository_ShowDeletionCascadesSettings(t *testing.T) {
	repo := getShowSettingsRepo()
	showID := createTestShow(t, repo)

	assert.NoError(t, repo.Upsert(model.ShowSettings{ShowID: showID}))

	showRepo := showRepository.NewShowRepository(repo.db)
	assert.NoError(t, showRepo.DeleteById(showID))

	_, err := repo.GetByShowID(showID)
	assert.Error(t, err)
}

func TestShowSettingsRepository_UpsertTx(t *testing.T) {
	repo := getShowSettingsRepo()
	showID := createTestShow(t, repo)

	tx, err := repo.db.Beginx()
	assert.NoError(t, err)

	err = repo.UpsertTx(tx, model.ShowSettings{
		ShowID:     showID,
		ShowType:   model.ShowTypeAnime,
		UseAliases: true,
		OnlyLatin:  false,
	})
	assert.NoError(t, err)
	assert.NoError(t, tx.Commit())

	got, err := repo.GetByShowID(showID)
	assert.NoError(t, err)
	assert.Equal(t, model.ShowTypeAnime, got.ShowType)
	assert.True(t, got.UseAliases)
	assert.False(t, got.OnlyLatin)
}
