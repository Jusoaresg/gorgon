package service

import (
	"log/slog"
	"testing"
	"time"

	"github.com/jusoaresg/gorgon/config"
	configRepo "github.com/jusoaresg/gorgon/internal/config/repository"
	configService "github.com/jusoaresg/gorgon/internal/config/service"
	episodeRepo "github.com/jusoaresg/gorgon/internal/episode/repository"
	seasonRepo "github.com/jusoaresg/gorgon/internal/season/repository"
	showRepo "github.com/jusoaresg/gorgon/internal/show/repository"
	"github.com/jusoaresg/gorgon/pkg/schemas"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDailySummary_TimezoneAware(t *testing.T) {
	db := testutils.GetTestDB()

	// Configure Gorgon timezone to America/Sao_Paulo (UTC-3)
	cfgRepo := configRepo.NewAppConfigRepository(db, false)
	cfgSvc, err := configService.NewConfigService(cfgRepo, slog.Default())
	require.NoError(t, err)

	tz := "America/Sao_Paulo"
	_, err = cfgSvc.Update(&schemas.UpdateConfigInput{
		Timezone: &tz,
	})
	require.NoError(t, err)
	config.SetConfigService(cfgSvc)

	loc, err := time.LoadLocation("America/Sao_Paulo")
	require.NoError(t, err)

	sRepo := showRepo.NewShowRepository(db)
	seaRepo := seasonRepo.NewSeasonRepository(db)
	epRepo := episodeRepo.NewEpisodeRepository(db)

	show := testutils.MakeFakeShow()
	show.Name = "Saturday Show"
	showID, err := sRepo.Create(show)
	require.NoError(t, err)

	season := testutils.MakeFakeSeason()
	season.ShowID = showID
	seasonID, err := seaRepo.Create(season)
	require.NoError(t, err)

	// Saturday 2026-09-19 22:00 BRT (which is Sunday 2026-09-20 01:00:00 UTC)
	satAirTime := time.Date(2026, 9, 19, 22, 0, 0, 0, loc)
	epSat := testutils.MakeFakeEpisode()
	epSat.ShowID = showID
	epSat.SeasonID = seasonID
	epSat.Name = "Saturday Night Special"
	epSat.AirStamp = satAirTime.Unix()
	epSat.Season = 1
	epSat.Number = 1
	epSat.Tracking = "monitored"
	_, err = epRepo.Create(epSat)
	require.NoError(t, err)

	// Sunday 2026-09-20 20:00 BRT (which is Sunday 2026-09-20 23:00:00 UTC)
	sunAirTime := time.Date(2026, 9, 20, 20, 0, 0, 0, loc)
	epSun := testutils.MakeFakeEpisode()
	epSun.ShowID = showID
	epSun.SeasonID = seasonID
	epSun.Name = "Sunday Premiere"
	epSun.AirStamp = sunAirTime.Unix()
	epSun.Season = 1
	epSun.Number = 2
	epSun.Tracking = "monitored"
	_, err = epRepo.Create(epSun)
	require.NoError(t, err)

	// Skipped episode on Saturday - should be excluded
	epSkipped := testutils.MakeFakeEpisode()
	epSkipped.ShowID = showID
	epSkipped.SeasonID = seasonID
	epSkipped.Name = "Skipped Episode"
	epSkipped.AirStamp = satAirTime.Unix()
	epSkipped.Season = 1
	epSkipped.Number = 3
	epSkipped.Tracking = "skipped"
	_, err = epRepo.Create(epSkipped)
	require.NoError(t, err)

	// Test 1: Query at Saturday 21:00 BRT (which is Sunday 00:00:00 UTC on a server running UTC)
	serverNowUTC := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	summary, err := GetDailySummary(db, serverNowUTC)
	require.NoError(t, err)

	assert.Equal(t, "19/09", summary.Date)
	require.Len(t, summary.Episodes, 1)
	assert.Equal(t, "Saturday Night Special", summary.Episodes[0].EpisodeName)
	assert.Equal(t, "22:00", summary.Episodes[0].AirTime)

	// Test 2: GenerateDailySummaryText formatting
	text := GenerateDailySummaryText(db, serverNowUTC)
	assert.Contains(t, text, "19/09")
	assert.Contains(t, text, "Saturday Show")
	assert.Contains(t, text, "22:00")
	assert.NotContains(t, text, "Sunday Premiere")
	assert.NotContains(t, text, "Skipped Episode")

	// Test 3: Query on Sunday 21:00 BRT (which is Monday 00:00:00 UTC)
	sundayNowUTC := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	summarySun, err := GetDailySummary(db, sundayNowUTC)
	require.NoError(t, err)

	assert.Equal(t, "20/09", summarySun.Date)
	require.Len(t, summarySun.Episodes, 1)
	assert.Equal(t, "Sunday Premiere", summarySun.Episodes[0].EpisodeName)
	assert.Equal(t, "20:00", summarySun.Episodes[0].AirTime)
}
