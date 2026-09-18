package service

import (
	"testing"

	"github.com/jusoaresg/gorgon/internal/filter"
	filterProfileModel "github.com/jusoaresg/gorgon/internal/filter_profile/model"
	filterProfileRepository "github.com/jusoaresg/gorgon/internal/filter_profile/repository"
	filterSettingsModel "github.com/jusoaresg/gorgon/internal/filter_settings/model"
	filterSettingsRepository "github.com/jusoaresg/gorgon/internal/filter_settings/repository"
	showAliasModel "github.com/jusoaresg/gorgon/internal/show_aliases/model"
	showAliasRepository "github.com/jusoaresg/gorgon/internal/show_aliases/repository"
	showModel "github.com/jusoaresg/gorgon/internal/show/model"
	showRepository "github.com/jusoaresg/gorgon/internal/show/repository"
	showSearchPatternsRepository "github.com/jusoaresg/gorgon/internal/show_search_patterns/repository"
	showSettingsModel "github.com/jusoaresg/gorgon/internal/show_settings/model"
	showSettingsRepository "github.com/jusoaresg/gorgon/internal/show_settings/repository"
	"github.com/jusoaresg/gorgon/testutils"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createResolverShow(t *testing.T, db *sqlx.DB) showModel.Show {
	t.Helper()
	show := testutils.MakeFakeShow()
	id, err := showRepository.NewShowRepository(db).Create(show)
	require.NoError(t, err)
	show.ID = id
	return show
}

func createAliases(t *testing.T, db *sqlx.DB, showID int64, aliases ...string) {
	t.Helper()
	repo := showAliasRepository.NewShowAliasesRepository(db)
	for _, alias := range aliases {
		_, err := repo.Create(showAliasModel.ShowAlias{
			ShowID: showID,
			Alias:  alias,
			Source: "tvmaze",
		})
		require.NoError(t, err)
	}
}

func TestResolveSettings_FallsBackToGlobal(t *testing.T) {
	db := testutils.GetTestDB()
	show := createResolverShow(t, db)

	settings, err := ResolveSettings(db, show.ID)
	require.NoError(t, err)
	assert.True(t, settings.UseAliases)
	assert.True(t, settings.OnlyLatin)
	assert.Nil(t, settings.FilterProfileID)
}

func TestResolveSettings_PerShowOverridesGlobal(t *testing.T) {
	db := testutils.GetTestDB()
	show := createResolverShow(t, db)

	global := filterSettingsModel.FilterSettings{UseAliases: true, OnlyLatin: false}
	settingsRepo := filterSettingsRepository.NewFilterSettingsRepository(db)
	require.NoError(t, settingsRepo.Save(global))

	showSettingsRepo := showSettingsRepository.NewShowSettingsRepository(db)
	require.NoError(t, showSettingsRepo.Upsert(showSettingsModel.ShowSettings{
		ShowID:     show.ID,
		UseAliases: false,
		OnlyLatin:  true,
	}))

	settings, err := ResolveSettings(db, show.ID)
	require.NoError(t, err)
	assert.False(t, settings.UseAliases)
	assert.True(t, settings.OnlyLatin)
}

func TestResolveSettings_PerShowProfileWins(t *testing.T) {
	db := testutils.GetTestDB()
	show := createResolverShow(t, db)

	profileRepo := filterProfileRepository.NewFilterProfileRepository(db)
	globalProfileID, err := profileRepo.Create(filterProfileModel.FilterProfile{Name: "Global"}, nil)
	require.NoError(t, err)
	showProfileID, err := profileRepo.Create(filterProfileModel.FilterProfile{Name: "Show"}, nil)
	require.NoError(t, err)

	global := filterSettingsModel.FilterSettings{DefaultFilterProfileID: &globalProfileID, UseAliases: true, OnlyLatin: true}
	settingsRepo := filterSettingsRepository.NewFilterSettingsRepository(db)
	require.NoError(t, settingsRepo.Save(global))

	showSettingsRepo := showSettingsRepository.NewShowSettingsRepository(db)
	require.NoError(t, showSettingsRepo.Upsert(showSettingsModel.ShowSettings{
		ShowID:          show.ID,
		FilterProfileID: &showProfileID,
		UseAliases:      true,
		OnlyLatin:       true,
	}))

	settings, err := ResolveSettings(db, show.ID)
	require.NoError(t, err)
	assert.NotNil(t, settings.FilterProfileID)
	assert.Equal(t, showProfileID, *settings.FilterProfileID)
}

func TestResolveProfile_NilWhenNoProfile(t *testing.T) {
	profile, err := ResolveProfile(testutils.GetTestDB(), EffectiveSettings{})
	require.NoError(t, err)
	assert.Nil(t, profile)
}

func TestResolveProfile_CombinesShowSearchPatterns(t *testing.T) {
	db := testutils.GetTestDB()
	repo := filterProfileRepository.NewFilterProfileRepository(db)

	id, err := repo.Create(filterProfileModel.FilterProfile{Name: "HD"}, []filterProfileModel.FilterPattern{
		{Kind: filterProfileModel.KindSearch, Pattern: "{alias} 1080p"},
		{Kind: filterProfileModel.KindRequired, Pattern: "multisub"},
	})
	require.NoError(t, err)

	profile, err := ResolveProfile(db, EffectiveSettings{
		FilterProfileID: &id,
		SearchPatterns:  []string{"{alias} 4k", "{alias} UHD"},
	})
	require.NoError(t, err)
	require.NotNil(t, profile)
	assert.Equal(t, []string{"{alias} 1080p", "{alias} 4k", "{alias} UHD"}, profile.Search)
	assert.Equal(t, []string{"multisub"}, profile.Required)
}

func TestResolveProfile_ShowSearchPatternsWithoutProfile(t *testing.T) {
	db := testutils.GetTestDB()

	profile, err := ResolveProfile(db, EffectiveSettings{
		SearchPatterns: []string{"{alias} 4k"},
	})
	require.NoError(t, err)
	require.NotNil(t, profile)
	assert.Equal(t, []string{"{alias} 4k"}, profile.Search)
}

func TestResolveProfile_ProfileSearchUsedWhenNoShowPatterns(t *testing.T) {
	db := testutils.GetTestDB()
	repo := filterProfileRepository.NewFilterProfileRepository(db)

	id, err := repo.Create(filterProfileModel.FilterProfile{Name: "HD"}, []filterProfileModel.FilterPattern{
		{Kind: filterProfileModel.KindSearch, Pattern: "{alias} 1080p"},
	})
	require.NoError(t, err)

	profile, err := ResolveProfile(db, EffectiveSettings{FilterProfileID: &id})
	require.NoError(t, err)
	require.NotNil(t, profile)
	assert.Equal(t, []string{"{alias} 1080p"}, profile.Search)
}

func TestResolveSettings_LoadsShowSearchPatterns(t *testing.T) {
	db := testutils.GetTestDB()
	show := createResolverShow(t, db)

	patternsRepo := showSearchPatternsRepository.NewShowSearchPatternsRepository(db)
	require.NoError(t, patternsRepo.Replace(show.ID, []string{"{alias} 4k", "{alias} UHD"}))

	settings, err := ResolveSettings(db, show.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"{alias} 4k", "{alias} UHD"}, settings.SearchPatterns)
}

func TestResolveProfile_MapsPatterns(t *testing.T) {
	db := testutils.GetTestDB()
	repo := filterProfileRepository.NewFilterProfileRepository(db)

	id, err := repo.Create(filterProfileModel.FilterProfile{Name: "HD"}, []filterProfileModel.FilterPattern{
		{Kind: filterProfileModel.KindSearch, Pattern: "{alias} 1080p"},
		{Kind: filterProfileModel.KindRequired, Pattern: "multisub"},
		{Kind: filterProfileModel.KindRejected, Pattern: "hdtv"},
		{Kind: filterProfileModel.KindPreferred, Pattern: "web", Score: 30},
	})
	require.NoError(t, err)

	profile, err := ResolveProfile(db, EffectiveSettings{FilterProfileID: &id})
	require.NoError(t, err)
	require.NotNil(t, profile)
	assert.Equal(t, "HD", profile.Name)
	assert.Equal(t, []string{"{alias} 1080p"}, profile.Search)
	assert.Equal(t, []string{"multisub"}, profile.Required)
	assert.Equal(t, []string{"hdtv"}, profile.Rejected)
	assert.Len(t, profile.Preferred, 1)
	assert.Equal(t, "web", profile.Preferred[0].Pattern)
	assert.Equal(t, 30, profile.Preferred[0].Score)
}

func TestBuildContext_UseAliasesDisabled(t *testing.T) {
	db := testutils.GetTestDB()
	show := testutils.MakeFakeShow()
	show.Name = "Test Show"
	id, err := showRepository.NewShowRepository(db).Create(show)
	require.NoError(t, err)
	show.ID = id
	createAliases(t, db, show.ID, "DBZ", "Dragonball Z")

	ctx, err := BuildContext(db, show, 1, 4, EffectiveSettings{UseAliases: false, OnlyLatin: true})
	require.NoError(t, err)
	assert.Equal(t, "test show", ctx.Show)
	assert.Empty(t, ctx.Aliases)
}

func TestBuildContext_OnlyLatinFiltersNonLatin(t *testing.T) {
	db := testutils.GetTestDB()
	show := createResolverShow(t, db)
	createAliases(t, db, show.ID, "DBZ", "ドラゴンボール", "dragon ball z")

	ctx, err := BuildContext(db, show, 1, 4, EffectiveSettings{UseAliases: true, OnlyLatin: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"dbz", "dragon ball z"}, ctx.Aliases)
}

func TestBuildContext_OnlyLatinDisabledKeepsAll(t *testing.T) {
	db := testutils.GetTestDB()
	show := createResolverShow(t, db)
	createAliases(t, db, show.ID, "ドラゴンボール")

	ctx, err := BuildContext(db, show, 1, 4, EffectiveSettings{UseAliases: true, OnlyLatin: false})
	require.NoError(t, err)
	assert.Equal(t, []string{"ドラゴンボール"}, ctx.Aliases)
}

func TestBuildContext_ExcludesDuplicatesAndCanonical(t *testing.T) {
	db := testutils.GetTestDB()
	show := testutils.MakeFakeShow()
	show.Name = "Test Show"
	id, err := showRepository.NewShowRepository(db).Create(show)
	require.NoError(t, err)
	show.ID = id

	createAliases(t, db, show.ID, "Test Show", "test show", "dbz", "DBZ")

	ctx, err := BuildContext(db, show, 1, 4, EffectiveSettings{UseAliases: true, OnlyLatin: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"dbz"}, ctx.Aliases)
}

func TestSearchPatterns_FallsBackToDefault(t *testing.T) {
	patterns := SearchPatterns(nil)
	assert.Equal(t, []string{"{alias} S{season:00}E{episode:00}"}, patterns)
}

func TestSearchPatterns_PrependsDefault(t *testing.T) {
	profile := &filter.Profile{Search: []string{"{alias} 4k", "{alias} UHD"}}
	patterns := SearchPatterns(profile)
	assert.Equal(t, []string{"{alias} S{season:00}E{episode:00}", "{alias} 4k", "{alias} UHD"}, patterns)
}

func TestSearchPatterns_NoDuplicateDefault(t *testing.T) {
	profile := &filter.Profile{Search: []string{"{alias} 4k", "{alias} S{season:00}E{episode:00}"}}
	patterns := SearchPatterns(profile)
	assert.Equal(t, []string{"{alias} 4k", "{alias} S{season:00}E{episode:00}"}, patterns)
}

func TestSearchPatterns_AnimeDefaults(t *testing.T) {
	patterns := SearchPatterns(nil, "anime")
	assert.Equal(t, []string{
		"{alias} - {episode:00}",
		"{alias} S{season} - {episode:00}",
		"{alias} S{season:00}E{episode:00}",
	}, patterns)
}


func TestResolveProfile_DedupesCombinedPatterns(t *testing.T) {
	db := testutils.GetTestDB()
	repo := filterProfileRepository.NewFilterProfileRepository(db)

	id, err := repo.Create(filterProfileModel.FilterProfile{Name: "HD"}, []filterProfileModel.FilterPattern{
		{Kind: filterProfileModel.KindSearch, Pattern: "{alias}"},
	})
	require.NoError(t, err)

	profile, err := ResolveProfile(db, EffectiveSettings{
		FilterProfileID: &id,
		SearchPatterns:  []string{"{alias}", "{alias} 4k"},
	})
	require.NoError(t, err)
	require.NotNil(t, profile)
	assert.Equal(t, []string{"{alias}", "{alias} 4k"}, profile.Search)
}

func TestDedupeStrings(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"}, dedupeStrings([]string{"a", "b", "a", "c", "b"}))
	assert.Equal(t, []string{"a"}, dedupeStrings([]string{"a", "a"}))
	assert.Nil(t, dedupeStrings(nil))
}
