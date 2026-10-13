package repository

import (
	"testing"
	"time"

	"github.com/jusoaresg/gorgon/internal/blacklist/model"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlacklistRepository_CreateAndListActive(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	showRepo := showRepository.NewShowRepository(db)
	showID, err := showRepo.Create(testutils.MakeFakeShow())
	require.NoError(t, err)

	repo := NewBlacklistRepository(db)
	now := time.Now().Unix()
	future := now + 3600
	past := now - 3600

	// 1. Permanent entry
	_, err = repo.Create(model.ReleaseBlacklist{
		ShowID:       showID,
		InfoHash:     "DEAD1234",
		ReleaseTitle: "[SubsPlease] Dandadan - 01 (1080p).mkv",
		Indexer:      "Nyaa",
		Reason:       "stalledDL timeout",
		CreatedAt:    now,
	})
	require.NoError(t, err)

	// 2. Active temporary entry
	_, err = repo.Create(model.ReleaseBlacklist{
		ShowID:       showID,
		InfoHash:     "TEMP5678",
		ReleaseTitle: "[Erai-raws] Dandadan - 01 [1080p].mkv",
		Indexer:      "Nyaa",
		Reason:       "metaDL timeout",
		CreatedAt:    now,
		ExpiresAt:    &future,
	})
	require.NoError(t, err)

	// 3. Expired temporary entry
	_, err = repo.Create(model.ReleaseBlacklist{
		ShowID:       showID,
		InfoHash:     "EXPIRED999",
		ReleaseTitle: "[Old] Dandadan - 01.mkv",
		Indexer:      "Nyaa",
		Reason:       "metaDL timeout",
		CreatedAt:    past - 3600,
		ExpiresAt:    &past,
	})
	require.NoError(t, err)

	active, err := repo.ListActiveByShowID(showID, now)
	require.NoError(t, err)
	require.Len(t, active, 2)

	hashes := []string{active[0].InfoHash, active[1].InfoHash}
	assert.Contains(t, hashes, "DEAD1234")
	assert.Contains(t, hashes, "TEMP5678")
}

func TestBlacklistRepository_DeleteByHashOrTitle(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	showRepo := showRepository.NewShowRepository(db)
	showID, err := showRepo.Create(testutils.MakeFakeShow())
	require.NoError(t, err)

	repo := NewBlacklistRepository(db)
	now := time.Now().Unix()

	_, err = repo.Create(model.ReleaseBlacklist{
		ShowID:       showID,
		InfoHash:     "ABCDEF123456",
		ReleaseTitle: "Severance.S02E01.1080p.WEB-DL-FLUX",
		Reason:       "stalled",
		CreatedAt:    now,
	})
	require.NoError(t, err)

	// Delete by case-insensitive hash
	err = repo.DeleteByHashOrTitle(showID, "abcdef123456", "")
	require.NoError(t, err)

	active, err := repo.ListActiveByShowID(showID, now)
	require.NoError(t, err)
	assert.Empty(t, active)

	// Re-insert without hash and delete by normalized title
	_, err = repo.Create(model.ReleaseBlacklist{
		ShowID:       showID,
		ReleaseTitle: "Severance.S02E01.1080p.WEB-DL-FLUX",
		Reason:       "stalled",
		CreatedAt:    now,
	})
	require.NoError(t, err)

	err = repo.DeleteByHashOrTitle(showID, "", "Severance S02E01 1080p WEB DL FLUX")
	require.NoError(t, err)

	active, err = repo.ListActiveByShowID(showID, now)
	require.NoError(t, err)
	assert.Empty(t, active)
}
