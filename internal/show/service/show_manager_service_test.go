package service

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	seasonModel "github.com/jusoaresg/gorgon/internal/season/model"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	showModel "github.com/jusoaresg/gorgon/internal/show/model"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	showAliasModel "github.com/jusoaresg/gorgon/internal/show_aliases/model"
	showAliasRepository "github.com/jusoaresg/gorgon/internal/show_aliases/repository"
	"github.com/jusoaresg/gorgon/pkg/schemas/dtos"
	"github.com/jusoaresg/gorgon/testutils"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type updateShowWithRelationsFixture struct {
	db     *sqlx.DB
	svc    *ShowManagerService
	showID int64
}

const seedShowTvMazeID int64 = 92620

func setupUpdateShowTest(t *testing.T) updateShowWithRelationsFixture {
	t.Helper()

	db := testutils.GetTestDB()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewShowManagerService(logger, db)

	rating := 8.5
	showID, err := showRepository.NewShowRepository(db).Create(showModel.Show{
		TvMazeID:      seedShowTvMazeID,
		Name:          "Old Name",
		Type:          "Scripted",
		Language:      "English",
		Status:        "Running",
		Premiered:     "2010-01-01",
		Ended:         "",
		Rating:        &rating,
		Summary:       "<p>old summary</p>",
		Updated:       100,
		TheTvDBD:      1,
		ImageMedium:   "http://example.com/medium.jpg",
		ImageOriginal: "http://example.com/original.jpg",
		Genres:        "Drama",
	})
	require.NoError(t, err)

	seasonRepo := seasonRepository.NewSeasonRepository(db)
	season1ID, err := seasonRepo.Create(seasonModel.Season{ShowID: showID, Number: 1})
	require.NoError(t, err)
	_, err = seasonRepo.Create(seasonModel.Season{ShowID: showID, Number: 2})
	require.NoError(t, err)

	_, err = episodeRepository.NewEpisodeRepository(db).Create(episodeModel.Episode{
		ShowID:   showID,
		SeasonID: season1ID,
		Name:     "Old Episode Name",
		Summary:  "<p>old episode</p>",
		Type:     "scripted",
		Number:   1,
		Season:   1,
		AirStamp: 1262304000,
		Tracking: episodeModel.TrackingWanted,
	})
	require.NoError(t, err)

	showAliasRepo := showAliasRepository.NewShowAliasesRepository(db)
	_, err = showAliasRepo.Create(showAliasModel.ShowAlias{
		ShowID:  showID,
		Alias:   "Old Alias",
		Country: "us",
		Source:  "user",
	})
	require.NoError(t, err)

	return updateShowWithRelationsFixture{db: db, svc: svc, showID: showID}
}

func buildUpdateShowDTO(t *testing.T, name string, akasJSON string) dtos.ShowDto {
	t.Helper()

	payload := `{
		"id": 92620,
		"name": "` + name + `",
		"type": "Scripted",
		"language": "English",
		"status": "Running",
		"premiered": "2010-01-01",
		"summary": "<p>new summary</p>",
		"updated": 200,
		"_embedded": {"akas": ` + akasJSON + `}
	}`

	var showDTO dtos.ShowDto
	require.NoError(t, json.Unmarshal([]byte(payload), &showDTO))
	return showDTO
}

func TestUpdateShowWithRelations_NewSeasonDoesNotViolateForeignKey(t *testing.T) {
	fx := setupUpdateShowTest(t)

	showDTO := buildUpdateShowDTO(t, "New Name", `[
		{"name": "Old Alias", "country": {"code": "br"}},
		{"name": "Brand New Alias", "country": {"code": "fr"}}
	]`)
	seasonsDTO := []dtos.SeasonDto{
		{Number: 1},
		{Number: 2},
		{Number: 3},
	}
	episodesDTO := []dtos.EpisodeDto{
		{Name: "Updated Episode Name", Season: 1, Number: 1, Type: "scripted", AirStamp: "2024-03-01T00:00:00Z", Summary: "<p>new episode</p>"},
		{Name: "Season 3 Premiere", Season: 3, Number: 1, Type: "scripted", AirStamp: "2026-09-01T00:00:00Z", Summary: "<p>premiere</p>"},
	}

	err := fx.svc.UpdateShowWithRelations(showDTO, seasonsDTO, episodesDTO)
	require.NoError(t, err)

	var seasonNumbers []int
	err = fx.db.Select(&seasonNumbers, `SELECT season_number FROM seasons WHERE show_id = ? ORDER BY season_number`, fx.showID)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, seasonNumbers, "new season must be created with valid show_id")

	var newSeasonShowID int64
	err = fx.db.Get(&newSeasonShowID, `SELECT show_id FROM seasons WHERE show_id = ? AND season_number = 3`, fx.showID)
	require.NoError(t, err)
	assert.Equal(t, fx.showID, newSeasonShowID)

	var aliases []showAliasModel.ShowAlias
	err = fx.db.Select(&aliases, `SELECT id, show_id, alias, country, source FROM show_aliases WHERE show_id = ? ORDER BY id`, fx.showID)
	require.NoError(t, err)
	require.Len(t, aliases, 2)

	aliasByName := make(map[string]showAliasModel.ShowAlias)
	for _, a := range aliases {
		aliasByName[a.Alias] = a
	}

	newAlias, ok := aliasByName["Brand New Alias"]
	require.True(t, ok, "new alias should have been created")
	assert.Equal(t, fx.showID, newAlias.ShowID)
	assert.Equal(t, "fr", newAlias.Country)
	assert.Equal(t, "tvmaze", newAlias.Source)

	updatedAlias, ok := aliasByName["Old Alias"]
	require.True(t, ok, "existing alias must be kept")
	assert.Equal(t, "br", updatedAlias.Country, "country change on existing alias must be persisted")

	var updatedEpisodeName string
	err = fx.db.Get(&updatedEpisodeName, `SELECT name FROM episodes WHERE show_id = ? AND season = 1 AND number = 1`, fx.showID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Episode Name", updatedEpisodeName)

	var newEpisode struct {
		SeasonID int64  `db:"season_id"`
		Name     string `db:"name"`
	}
	err = fx.db.Get(&newEpisode,
		`SELECT season_id, name FROM episodes WHERE show_id = ? AND season = 3 AND number = 1`, fx.showID)
	require.NoError(t, err)
	assert.Equal(t, "Season 3 Premiere", newEpisode.Name)
	assert.NotZero(t, newEpisode.SeasonID)

	var newName string
	err = fx.db.Get(&newName, `SELECT name FROM shows WHERE id = ?`, fx.showID)
	require.NoError(t, err)
	assert.Equal(t, "New Name", newName)
}

func TestUpdateShowWithRelations_MissingSeasonIsAutoCreated(t *testing.T) {
	fx := setupUpdateShowTest(t)

	showDTO := buildUpdateShowDTO(t, "Updated Name", `[{"name": "Some Alias", "country": {"code": "us"}}]`)
	seasonsDTO := []dtos.SeasonDto{{Number: 7}}
	episodesDTO := []dtos.EpisodeDto{
		{Name: "Season 8 Episode", Season: 8, Number: 1, Type: "scripted", AirStamp: "2026-01-01T00:00:00Z"},
	}

	err := fx.svc.UpdateShowWithRelations(showDTO, seasonsDTO, episodesDTO)
	require.NoError(t, err, "episode referencing missing season should auto-create season and succeed")

	var count int
	err = fx.db.Get(&count, `SELECT COUNT(*) FROM seasons WHERE show_id = ? AND season_number = 8`, fx.showID)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "season 8 should be auto-created")

	err = fx.db.Get(&count, `SELECT COUNT(*) FROM episodes WHERE show_id = ? AND season = 8 AND number = 1`, fx.showID)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "episode in auto-created season should be created")
}

func TestUpdateShowWithRelations_DeletedEpisode_RemovedWhenNotDownloadedOrSnatched(t *testing.T) {
	trackings := []string{
		episodeModel.TrackingWanted,
		episodeModel.TrackingMissing,
		episodeModel.TrackingSkipped,
	}

	for _, tracking := range trackings {
		t.Run("deletes episode with tracking "+tracking, func(t *testing.T) {
			fx := setupUpdateShowTest(t)

			// Update the initial episode (S01E01) to the given tracking status
			_, err := fx.db.Exec(`UPDATE episodes SET tracking = ? WHERE show_id = ? AND season = 1 AND number = 1`, tracking, fx.showID)
			require.NoError(t, err)

			showDTO := buildUpdateShowDTO(t, "Show With Episode Removed", `[]`)
			seasonsDTO := []dtos.SeasonDto{{Number: 1}, {Number: 2}}
			// TVmaze now only returns S02E01; S01E01 was removed from TVmaze!
			episodesDTO := []dtos.EpisodeDto{
				{Name: "Season 2 Episode 1", Season: 2, Number: 1, Type: "scripted", AirStamp: "2026-05-01T00:00:00Z"},
			}

			err = fx.svc.UpdateShowWithRelations(showDTO, seasonsDTO, episodesDTO)
			require.NoError(t, err)

			// S01E01 should have been deleted from the database
			var count int
			err = fx.db.Get(&count, `SELECT COUNT(*) FROM episodes WHERE show_id = ? AND season = 1 AND number = 1`, fx.showID)
			require.NoError(t, err)
			assert.Zero(t, count, "episode with tracking %s should be removed when absent from TVmaze", tracking)

			// S02E01 should be present
			err = fx.db.Get(&count, `SELECT COUNT(*) FROM episodes WHERE show_id = ? AND season = 2 AND number = 1`, fx.showID)
			require.NoError(t, err)
			assert.Equal(t, 1, count, "new TVmaze episode S02E01 must exist")
		})
	}
}

func TestUpdateShowWithRelations_DeletedEpisode_PreservedWhenSnatchedOrDownloaded(t *testing.T) {
	trackings := []string{
		episodeModel.TrackingSnatched,
		episodeModel.TrackingDownloaded,
	}

	for _, tracking := range trackings {
		t.Run("preserves episode with tracking "+tracking, func(t *testing.T) {
			fx := setupUpdateShowTest(t)

			// Update initial episode (S01E01) to snatched or downloaded
			_, err := fx.db.Exec(`UPDATE episodes SET tracking = ? WHERE show_id = ? AND season = 1 AND number = 1`, tracking, fx.showID)
			require.NoError(t, err)

			showDTO := buildUpdateShowDTO(t, "Show With Episode Removed From TVmaze", `[]`)
			seasonsDTO := []dtos.SeasonDto{{Number: 1}, {Number: 2}}
			// TVmaze does not return S01E01
			episodesDTO := []dtos.EpisodeDto{
				{Name: "Season 2 Episode 1", Season: 2, Number: 1, Type: "scripted", AirStamp: "2026-05-01T00:00:00Z"},
			}

			err = fx.svc.UpdateShowWithRelations(showDTO, seasonsDTO, episodesDTO)
			require.NoError(t, err)

			// S01E01 must be PRESERVED
			var count int
			err = fx.db.Get(&count, `SELECT COUNT(*) FROM episodes WHERE show_id = ? AND season = 1 AND number = 1`, fx.showID)
			require.NoError(t, err)
			assert.Equal(t, 1, count, "episode with tracking %s must NOT be deleted even if removed from TVmaze", tracking)
		})
	}
}
