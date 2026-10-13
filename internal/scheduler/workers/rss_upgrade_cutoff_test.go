package workers

import (
	"testing"

	prowlarrSchema "github.com/jusoaresg/gorgon/external/prowlarr/schema"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	episodeRepository "github.com/jusoaresg/gorgon/internal/episode/repository"
	episodeService "github.com/jusoaresg/gorgon/internal/episode/service"
	episodeTorrentModel "github.com/jusoaresg/gorgon/internal/episode_torrent/model"
	episodeTorrentRepository "github.com/jusoaresg/gorgon/internal/episode_torrent/repository"
	filterProfileModel "github.com/jusoaresg/gorgon/internal/filter_profile/model"
	filterProfileRepository "github.com/jusoaresg/gorgon/internal/filter_profile/repository"
	filterSettingsModel "github.com/jusoaresg/gorgon/internal/filter_settings/model"
	filterSettingsRepository "github.com/jusoaresg/gorgon/internal/filter_settings/repository"
	seasonModel "github.com/jusoaresg/gorgon/internal/season/model"
	seasonRepository "github.com/jusoaresg/gorgon/internal/season/repository"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	showSettingsModel "github.com/jusoaresg/gorgon/internal/show_settings/model"
	showSettingsRepository "github.com/jusoaresg/gorgon/internal/show_settings/repository"
	"github.com/jusoaresg/gorgon/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDownloader struct {
	downloadCalled bool
	upgradeCalled  bool
	lastScore      int
	lastTitle      string
}

func (m *mockDownloader) DownloadEpisode(ep episodeModel.Episode, response prowlarrSchema.SearchResponse, score ...int) error {
	m.downloadCalled = true
	if len(score) > 0 {
		m.lastScore = score[0]
	}
	m.lastTitle = response.Title
	return nil
}

func (m *mockDownloader) DownloadEpisodeTorrent(ep episodeModel.Episode, response prowlarrSchema.SearchResponse, score ...int) error {
	return m.DownloadEpisode(ep, response, score...)
}

func (m *mockDownloader) UpgradeEpisode(ep episodeModel.Episode, response prowlarrSchema.SearchResponse, newScore int, upgrade ...episodeService.UpgradeInfo) error {
	m.upgradeCalled = true
	m.lastScore = newScore
	m.lastTitle = response.Title
	return nil
}

func setupProcessorTest(t *testing.T, cutoffScore int) (RssReleaseProcessor, *mockDownloader, int64, episodeModel.Episode) {
	db := testutils.GetTestDB()

	showRepo := showRepository.NewShowRepository(db)
	seasonRepo := seasonRepository.NewSeasonRepository(db)
	episodeRepo := episodeRepository.NewEpisodeRepository(db)
	profileRepo := filterProfileRepository.NewFilterProfileRepository(db)
	filterSetRepo := filterSettingsRepository.NewFilterSettingsRepository(db)
	showSetRepo := showSettingsRepository.NewShowSettingsRepository(db)

	show := testutils.MakeFakeShow()
	show.Name = "Frieren"
	showID, err := showRepo.Create(show)
	require.NoError(t, err)

	seasonID, err := seasonRepo.Create(seasonModel.Season{
		ShowID: showID,
		Number: 1,
	})
	require.NoError(t, err)

	epID, err := episodeRepo.Create(episodeModel.Episode{
		ShowID:   showID,
		SeasonID: seasonID,
		Name:     "Episode 5",
		Season:   1,
		Number:   5,
		Tracking: episodeModel.TrackingDownloaded,
	})
	require.NoError(t, err)

	profileID, err := profileRepo.Create(filterProfileModel.FilterProfile{
		Name:              "Default",
		UpgradeUntilScore: cutoffScore,
	}, []filterProfileModel.FilterPattern{
		{Kind: filterProfileModel.KindPreferred, Pattern: "1080p", Score: 30},
	})
	require.NoError(t, err)

	_ = filterSetRepo.Save(filterSettingsModel.FilterSettings{
		DefaultFilterProfileID: &profileID,
		UseAliases:             true,
		OnlyLatin:              true,
	})
	_ = showSetRepo.Upsert(showSettingsModel.ShowSettings{
		ShowID:          showID,
		FilterProfileID: &profileID,
		ShowType:        showSettingsModel.ShowTypeStandard,
		UseAliases:      true,
		OnlyLatin:       true,
	})

	mock := &mockDownloader{}
	processor := RssReleaseProcessor{
		db:                 db,
		showRepo:           showRepo,
		seasonRepo:         seasonRepo,
		episodeTorrentRepo: episodeTorrentRepository.NewEpisodeTorrentRepository(db),
		downloader:         mock,
	}

	ep := episodeModel.Episode{
		ID:       epID,
		ShowID:   showID,
		Season:   1,
		Number:   5,
		Tracking: episodeModel.TrackingDownloaded,
	}

	return processor, mock, showID, ep
}

func TestRssProcessRelease_UpgradeWhenUnderCutoffAndHigherScore(t *testing.T) {
	processor, mock, _, ep := setupProcessorTest(t, 60)
	defer processor.db.Close()

	epTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(processor.db)

	// Existing torrent snatched with score 20
	_, err := epTorrentRepo.Upsert(episodeTorrentModel.EpisodeTorrent{
		EpisodeId: ep.ID,
		Hash:      "existing_hash",
		Score:     20,
	})
	require.NoError(t, err)

	// Release with 1080p (+30 preferred)
	releases := []prowlarrSchema.SearchResponse{
		{
			Title:    "Frieren S01E05 1080p WEB-DL",
			Filename: "Frieren.S01E05.1080p.mkv",
			InfoHash: "new_higher_hash",
			Guid:     "magnet:?xt=urn:btih:new_higher_hash",
			Seeders:  10,
		},
	}

	err = processor.RssProcessRelease(ep, releases)
	require.NoError(t, err)

	assert.True(t, mock.upgradeCalled, "Upgrade should be called when score is higher and under cutoff")
	assert.False(t, mock.downloadCalled)
	assert.Greater(t, mock.lastScore, 20)
}

func TestRssProcessRelease_SkipWhenCurrentScoreReachedCutoff(t *testing.T) {
	processor, mock, _, ep := setupProcessorTest(t, 50)
	defer processor.db.Close()

	epTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(processor.db)

	// Existing torrent already reached cutoff score 50
	_, err := epTorrentRepo.Upsert(episodeTorrentModel.EpisodeTorrent{
		EpisodeId: ep.ID,
		Hash:      "cutoff_hash",
		Score:     50,
	})
	require.NoError(t, err)

	releases := []prowlarrSchema.SearchResponse{
		{
			Title:    "Frieren S01E05 1080p WEB-DL",
			Filename: "Frieren.S01E05.1080p.mkv",
			InfoHash: "even_better_hash",
			Guid:     "magnet:?xt=urn:btih:even_better_hash",
		},
	}

	err = processor.RssProcessRelease(ep, releases)
	require.NoError(t, err)

	assert.False(t, mock.upgradeCalled, "Upgrade should NOT be called if current score >= cutoff")
	assert.False(t, mock.downloadCalled)
}

func TestRssProcessRelease_SkipWhenCandidateScoreIsLowerOrEqual(t *testing.T) {
	processor, mock, _, ep := setupProcessorTest(t, 100)
	defer processor.db.Close()

	epTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(processor.db)

	ep.Tracking = episodeModel.TrackingSnatched

	// Current score is 80 (under cutoff 100)
	_, err := epTorrentRepo.Upsert(episodeTorrentModel.EpisodeTorrent{
		EpisodeId: ep.ID,
		Hash:      "high_hash",
		Score:     80,
	})
	require.NoError(t, err)

	// Release with lower score (no 1080p pattern match)
	releases := []prowlarrSchema.SearchResponse{
		{
			Title:    "Frieren S01E05 480p HDTV",
			Filename: "Frieren.S01E05.480p.mkv",
			InfoHash: "low_hash",
			Guid:     "magnet:?xt=urn:btih:low_hash",
		},
	}

	err = processor.RssProcessRelease(ep, releases)
	require.NoError(t, err)

	assert.False(t, mock.upgradeCalled, "Upgrade should NOT be called if candidate score <= existing score")
	assert.False(t, mock.downloadCalled)
}

func TestRssProcessRelease_SkipUpgradeWhenCutoffIsZero(t *testing.T) {
	processor, mock, _, ep := setupProcessorTest(t, 0)
	defer processor.db.Close()

	epTorrentRepo := episodeTorrentRepository.NewEpisodeTorrentRepository(processor.db)

	_, err := epTorrentRepo.Upsert(episodeTorrentModel.EpisodeTorrent{
		EpisodeId: ep.ID,
		Hash:      "existing_hash",
		Score:     10,
	})
	require.NoError(t, err)

	releases := []prowlarrSchema.SearchResponse{
		{
			Title:    "Frieren S01E05 1080p WEB-DL",
			Filename: "Frieren.S01E05.1080p.mkv",
			InfoHash: "candidate_hash",
			Guid:     "magnet:?xt=urn:btih:candidate_hash",
		},
	}

	err = processor.RssProcessRelease(ep, releases)
	require.NoError(t, err)

	assert.False(t, mock.upgradeCalled, "Upgrade should NOT be called when cutoff is disabled (0)")
	assert.False(t, mock.downloadCalled)
}
