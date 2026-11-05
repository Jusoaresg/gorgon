package service

import (
	ps "github.com/jusoaresg/gorgon/external/prowlarr/schema"
	er "github.com/jusoaresg/gorgon/internal/episode/repository"
	seasonr "github.com/jusoaresg/gorgon/internal/season/repository"
	sr "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSnatchRejectsStaleEpisodeAfterDeletion(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()
	show, err := sr.NewShowRepository(db).Create(testutils.MakeFakeShow())
	require.NoError(t, err)
	season := testutils.MakeFakeSeason()
	season.ShowID = show
	seasonID, err := seasonr.NewSeasonRepository(db).Create(season)
	require.NoError(t, err)
	ep := testutils.MakeFakeEpisode()
	ep.ShowID = show
	ep.SeasonID = seasonID
	ep.Tracking = "wanted"
	ep.ID, err = er.NewEpisodeRepository(db).Create(ep)
	require.NoError(t, err)
	_, err = db.Exec("UPDATE episodes SET tracking='skipped' WHERE id=?", ep.ID)
	require.NoError(t, err)
	require.Error(t, SnatchEpisode(&ep, ps.SearchResponse{InfoHash: "stale"}))
	saved, err := er.NewEpisodeRepository(db).GetByID(ep.ID)
	require.NoError(t, err)
	require.Equal(t, "skipped", saved.Tracking)
	var count int
	require.NoError(t, db.Get(&count, "SELECT count(*) FROM episode_torrents WHERE episode_id=?", ep.ID))
	require.Zero(t, count)
}
