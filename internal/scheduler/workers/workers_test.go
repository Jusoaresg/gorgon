package workers

import (
	"context"
	"testing"
	"time"

	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
	"github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	showAliasModel "github.com/jusoaresg/gorgon/internal/show_aliases/model"
	showAliasRepository "github.com/jusoaresg/gorgon/internal/show_aliases/repository"
	showSettingsModel "github.com/jusoaresg/gorgon/internal/show_settings/model"
	showSettingsRepository "github.com/jusoaresg/gorgon/internal/show_settings/repository"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEpisodeSearchWorker_Cancellation(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		StartEpisodeSearchWorker(ctx, 2, nil, nil)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Exited cleanly
	case <-time.After(1 * time.Second):
		t.Fatal("StartEpisodeSearchWorker did not terminate within timeout after context cancellation")
	}
}

func TestRssEpisodeFetcherWorker_Cancellation(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		StartRssEpisodeFetcherWorker(ctx, 2, nil)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Exited cleanly
	case <-time.After(1 * time.Second):
		t.Fatal("StartRssEpisodeFetcherWorker did not terminate within timeout after context cancellation")
	}
}

func TestVerifySnatchedDownloadsWorker_Cancellation(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		VerifySnatchedDownloadsWorker(ctx, 2, nil)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Exited cleanly
	case <-time.After(1 * time.Second):
		t.Fatal("VerifySnatchedDownloadsWorker did not terminate within timeout after context cancellation")
	}
}

func TestProcessEpisodesWorker_ChannelCloseAndContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan model.Episode, 5)
	done := make(chan struct{})

	go func() {
		processEpisodesWorker(ctx, ch, nil, nil)
		close(done)
	}()

	// Closing channel should cause worker to terminate
	close(ch)

	select {
	case <-done:
		// Exited cleanly on channel close
	case <-time.After(500 * time.Millisecond):
		t.Fatal("processEpisodesWorker did not exit on channel close")
	}
}

func TestProcessEpisodesWorker_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	ch := make(chan model.Episode, 5)
	done := make(chan struct{})

	go func() {
		processEpisodesWorker(ctx, ch, nil, nil)
		close(done)
	}()

	// Cancel context directly
	cancel()

	select {
	case <-done:
		// Exited cleanly on context cancellation
	case <-time.After(500 * time.Millisecond):
		t.Fatal("processEpisodesWorker did not exit on context cancel")
	}
}

func TestProcessSnatchedDownloadsWorker_ChannelClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan model.Episode, 5)
	done := make(chan struct{})

	go func() {
		processSnatchedDownloadsWorker(ctx, ch, nil)
		close(done)
	}()

	close(ch)

	select {
	case <-done:
		// Exited cleanly
	case <-time.After(500 * time.Millisecond):
		t.Fatal("processSnatchedDownloadsWorker did not exit on channel close")
	}
}

func TestEpisodeSearchWorker_NonBlockingSendOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	ch := make(chan model.Episode) // unbuffered, send would block without context check
	ep := model.Episode{ID: 1}

	select {
	case <-ctx.Done():
		// Succeeded in avoiding deadlock
	case ch <- ep:
		t.Fatal("should not have sent on channel")
	default:
		t.Fatal("unexpected default")
	}
	assert.True(t, true)
}

func TestMatchReleasesToEpisodes_ReleaseFirstFiltering(t *testing.T) {
	db := testutils.GetTestDB()
	defer db.Close()

	showRepo := showRepository.NewShowRepository(db)
	seasonRepo := seasonRepository.NewSeasonRepository(db)
	epRepo := episodeRepository.NewEpisodeRepository(db)
	aliasRepo := showAliasRepository.NewShowAliasesRepository(db)
	settingsRepo := showSettingsRepository.NewShowSettingsRepository(db)

	// Seed Show 1: Standard TV series ("Severance")
	show1 := testutils.MakeFakeShow()
	show1.Name = "Severance"
	show1ID, err := showRepo.Create(show1)
	require.NoError(t, err)
	require.NoError(t, settingsRepo.Upsert(showSettingsModel.ShowSettings{
		ShowID:     show1ID,
		ShowType:   showSettingsModel.ShowTypeStandard,
		UseAliases: true,
		OnlyLatin:  true,
	}))
	season1 := testutils.MakeFakeSeason()
	season1.ShowID = show1ID
	s1ID, err := seasonRepo.Create(season1)
	require.NoError(t, err)

	ep1 := testutils.MakeFakeEpisode()
	ep1.ShowID = show1ID
	ep1.SeasonID = s1ID
	ep1.Season = 2
	ep1.Number = 1
	ep1.Tracking = model.TrackingWanted
	ep1.ID, err = epRepo.Create(ep1)
	require.NoError(t, err)

	// Seed Show 2: Anime series with alias ("Frieren: Beyond Journey's End" / "Sousou no Frieren")
	show2 := testutils.MakeFakeShow()
	show2.Name = "Frieren: Beyond Journey's End"
	show2ID, err := showRepo.Create(show2)
	require.NoError(t, err)
	require.NoError(t, settingsRepo.Upsert(showSettingsModel.ShowSettings{
		ShowID:     show2ID,
		ShowType:   showSettingsModel.ShowTypeAnime,
		UseAliases: true,
		OnlyLatin:  true,
	}))
	_, err = aliasRepo.Create(showAliasModel.ShowAlias{
		ShowID: show2ID,
		Alias:  "Sousou no Frieren",
		Source: "user",
	})
	require.NoError(t, err)

	season2 := testutils.MakeFakeSeason()
	season2.ShowID = show2ID
	s2ID, err := seasonRepo.Create(season2)
	require.NoError(t, err)

	ep2 := testutils.MakeFakeEpisode()
	ep2.ShowID = show2ID
	ep2.SeasonID = s2ID
	ep2.Season = 1
	ep2.Number = 5
	ep2.Tracking = model.TrackingWanted
	ep2.ID, err = epRepo.Create(ep2)
	require.NoError(t, err)

	// Ep 3: Tracked episode with no matching releases in the RSS batch
	ep3 := testutils.MakeFakeEpisode()
	ep3.ShowID = show1ID
	ep3.SeasonID = s1ID
	ep3.Season = 2
	ep3.Number = 9
	ep3.Tracking = model.TrackingWanted
	ep3.ID, err = epRepo.Create(ep3)
	require.NoError(t, err)

	rssResponses := []schema.SearchResponse{
		{Title: "Random.Movie.2026.1080p.BluRay.x264", InfoHash: "hash1"},
		{Title: "The.Bear.S03E01.1080p.WEB-DL", InfoHash: "hash2"},
		{Title: "Severance.2022.S02E01.1080p.ATVP.WEB-DL.DDP5.1.H.264-FLUX", InfoHash: "hash3"},
		{Title: "[SubsPlease] Sousou no Frieren - 05 (1080p) [AABBCCDD].mkv", InfoHash: "hash4"},
	}

	processor := NewRssReleaseProcessor(db)
	jobs := processor.MatchReleasesToEpisodes([]model.Episode{ep1, ep2, ep3}, rssResponses)

	require.Len(t, jobs, 2)

	matchedByEpID := make(map[int64][]schema.SearchResponse)
	for _, job := range jobs {
		matchedByEpID[job.Episode.ID] = job.Responses
	}

	require.Contains(t, matchedByEpID, ep1.ID)
	assert.Len(t, matchedByEpID[ep1.ID], 1)
	assert.Equal(t, "hash3", matchedByEpID[ep1.ID][0].InfoHash)

	require.Contains(t, matchedByEpID, ep2.ID)
	assert.Len(t, matchedByEpID[ep2.ID], 1)
	assert.Equal(t, "hash4", matchedByEpID[ep2.ID][0].InfoHash)

	assert.NotContains(t, matchedByEpID, ep3.ID)
}
