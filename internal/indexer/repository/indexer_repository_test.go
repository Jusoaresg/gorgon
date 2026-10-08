package repository

import (
	"testing"

	"github.com/jusoaresg/gorgon/internal/indexer/model"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexerRepository_Create_And_GetById(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	repo := NewIndexerRepository(db)
	idx := model.Indexer{
		IndexerID:      42,
		Name:           "1337x",
		Enabled:        true,
		DefinitionName: "1337x-def",
		IndexerUrls:    "https://1337x.to",
		Language:       "en",
	}

	err := repo.Create(idx)
	require.NoError(t, err)

	list, err := repo.List()
	require.NoError(t, err)
	require.Len(t, list, 1)

	found, err := repo.GetById(int(list[0].ID))
	require.NoError(t, err)
	assert.Equal(t, 42, found.IndexerID)
	assert.Equal(t, "1337x", found.Name)
	assert.True(t, found.Enabled)
	assert.Equal(t, "1337x-def", found.DefinitionName)
	assert.Equal(t, "https://1337x.to", found.IndexerUrls)
	assert.Equal(t, "en", found.Language)
}

func TestIndexerRepository_GetById_NotFound(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	repo := NewIndexerRepository(db)
	_, err := repo.GetById(99999)
	assert.Error(t, err)
}

func TestIndexerRepository_List_Empty(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	repo := NewIndexerRepository(db)
	list, err := repo.List()
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestIndexerRepository_DeleteById(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	repo := NewIndexerRepository(db)
	idx := model.Indexer{
		IndexerID:      10,
		Name:           "RARBG",
		Enabled:        true,
		DefinitionName: "rarbg-def",
	}
	err := repo.Create(idx)
	require.NoError(t, err)

	list, err := repo.List()
	require.NoError(t, err)
	require.Len(t, list, 1)

	err = repo.DeleteById(int(list[0].ID))
	require.NoError(t, err)

	_, err = repo.GetById(int(list[0].ID))
	assert.Error(t, err)

	// Deleting again should return ErrIndexerNotFound
	err = repo.DeleteById(int(list[0].ID))
	assert.ErrorIs(t, err, ErrIndexerNotFound)
}
