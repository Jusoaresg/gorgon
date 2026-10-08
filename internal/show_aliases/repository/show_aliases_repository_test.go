package repository

import (
	"testing"

	"github.com/jusoaresg/gorgon/internal/show/model"
	showRepo "github.com/jusoaresg/gorgon/internal/show/repository"
	aliasModel "github.com/jusoaresg/gorgon/internal/show_aliases/model"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShowAliasesRepository_Create_And_GetByID(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	sRepo := showRepo.NewShowRepository(db)
	showID, err := sRepo.Create(model.Show{TvMazeID: 101, Name: "Show 101"})
	require.NoError(t, err)

	repo := NewShowAliasesRepository(db)
	alias := aliasModel.ShowAlias{
		ShowID:  showID,
		Alias:   "Alternative Name",
		Country: "US",
		Source:  "user",
	}

	id, err := repo.Create(alias)
	require.NoError(t, err)
	assert.NotZero(t, id)

	found, err := repo.GetByID(id)
	require.NoError(t, err)
	assert.Equal(t, id, found.ID)
	assert.Equal(t, showID, found.ShowID)
	assert.Equal(t, "Alternative Name", found.Alias)
	assert.Equal(t, "US", found.Country)
	assert.Equal(t, "user", found.Source)
}

func TestShowAliasesRepository_GetByID_NotFound(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	repo := NewShowAliasesRepository(db)
	_, err := repo.GetByID(99999)
	assert.Error(t, err)
}

func TestShowAliasesRepository_CreateTx_And_UpdateTx(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	sRepo := showRepo.NewShowRepository(db)
	showID, err := sRepo.Create(model.Show{TvMazeID: 102, Name: "Show 102"})
	require.NoError(t, err)

	repo := NewShowAliasesRepository(db)

	tx, err := db.Beginx()
	require.NoError(t, err)

	id, err := repo.CreateTx(tx, aliasModel.ShowAlias{
		ShowID:  showID,
		Alias:   "Tx Alias",
		Country: "GB",
		Source:  "tvmaze",
	})
	require.NoError(t, err)
	assert.NotZero(t, id)

	updateAlias := aliasModel.ShowAlias{
		ID:      id,
		Alias:   "Tx Alias Updated",
		Country: "UK",
	}
	err = repo.UpdateTx(tx, updateAlias)
	require.NoError(t, err)

	err = tx.Commit()
	require.NoError(t, err)

	found, err := repo.GetByID(id)
	require.NoError(t, err)
	assert.Equal(t, "Tx Alias Updated", found.Alias)
	assert.Equal(t, "UK", found.Country)
}

func TestShowAliasesRepository_UpdateTx_NotFound(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	repo := NewShowAliasesRepository(db)

	tx, err := db.Beginx()
	require.NoError(t, err)

	err = repo.UpdateTx(tx, aliasModel.ShowAlias{
		ID:    99999,
		Alias: "Ghost",
	})
	assert.ErrorIs(t, err, ErrAliasNotFound)
	_ = tx.Rollback()
}

func TestShowAliasesRepository_ListByShowID(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	sRepo := showRepo.NewShowRepository(db)
	showID, err := sRepo.Create(model.Show{TvMazeID: 103, Name: "Show 103"})
	require.NoError(t, err)

	repo := NewShowAliasesRepository(db)

	_, err = repo.Create(aliasModel.ShowAlias{ShowID: showID, Alias: "Alias A", Source: "user"})
	require.NoError(t, err)
	_, err = repo.Create(aliasModel.ShowAlias{ShowID: showID, Alias: "Alias B", Source: "tvmaze"})
	require.NoError(t, err)

	list, err := repo.ListByShowID(showID)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestShowAliasesRepository_DeleteByID(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	sRepo := showRepo.NewShowRepository(db)
	showID, err := sRepo.Create(model.Show{TvMazeID: 104, Name: "Show 104"})
	require.NoError(t, err)

	repo := NewShowAliasesRepository(db)
	id, err := repo.Create(aliasModel.ShowAlias{ShowID: showID, Alias: "To Delete", Source: "user"})
	require.NoError(t, err)

	err = repo.DeleteByID(id)
	require.NoError(t, err)

	_, err = repo.GetByID(id)
	assert.Error(t, err)

	// Deleting non-existent alias should return ErrAliasNotFound
	err = repo.DeleteByID(id)
	assert.ErrorIs(t, err, ErrAliasNotFound)
}
