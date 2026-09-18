package service

import (
	"database/sql"
	"errors"
	"regexp"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/internal/filter"
	filterProfileModel "github.com/jusoaresg/gorgon/internal/filter_profile/model"
	filterProfileRepository "github.com/jusoaresg/gorgon/internal/filter_profile/repository"
	filterSettingsRepository "github.com/jusoaresg/gorgon/internal/filter_settings/repository"
	showModel "github.com/jusoaresg/gorgon/internal/show/model"
	showAliasRepository "github.com/jusoaresg/gorgon/internal/show_aliases/repository"
	showSearchPatternsRepository "github.com/jusoaresg/gorgon/internal/show_search_patterns/repository"
	showSettingsModel "github.com/jusoaresg/gorgon/internal/show_settings/model"
	showSettingsRepository "github.com/jusoaresg/gorgon/internal/show_settings/repository"
	"github.com/jusoaresg/gorgon/utils"
)

var latinRegex = regexp.MustCompile(`^[a-zA-Z0-9\s\-_!?',.:]+$`)

// EffectiveSettings is the resolved filtering configuration for a show,
// merging per-show settings over the global defaults. A show_settings row
// fully overrides the global toggles and, when present, the profile. The
// per-show search patterns are combined with the profile's search patterns.
type EffectiveSettings struct {
	FilterProfileID *int64
	ShowType        string
	UseAliases      bool
	OnlyLatin       bool
	SearchPatterns  []string
}

func ResolveSettings(db *sqlx.DB, showID int64) (EffectiveSettings, error) {
	settingsRepo := filterSettingsRepository.NewFilterSettingsRepository(db)

	global, err := settingsRepo.Get()
	if err != nil {
		return EffectiveSettings{}, err
	}

	settings := EffectiveSettings{
		FilterProfileID: global.DefaultFilterProfileID,
		ShowType:        showSettingsModel.ShowTypeStandard,
		UseAliases:      global.UseAliases,
		OnlyLatin:       global.OnlyLatin,
	}

	showSettingsRepo := showSettingsRepository.NewShowSettingsRepository(db)
	showSettings, err := showSettingsRepo.GetByShowID(showID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return EffectiveSettings{}, err
		}
	} else {
		if showSettings.FilterProfileID != nil {
			settings.FilterProfileID = showSettings.FilterProfileID
		}
		if showSettings.ShowType != "" {
			settings.ShowType = showSettings.ShowType
		}
		settings.UseAliases = showSettings.UseAliases
		settings.OnlyLatin = showSettings.OnlyLatin
	}

	searchPatternsRepo := showSearchPatternsRepository.NewShowSearchPatternsRepository(db)
	searchPatterns, err := searchPatternsRepo.GetByShowID(showID)
	if err != nil {
		return EffectiveSettings{}, err
	}
	settings.SearchPatterns = searchPatterns

	return settings, nil
}

func ResolveProfile(db *sqlx.DB, settings EffectiveSettings) (*filter.Profile, error) {
	profile := &filter.Profile{}

	if settings.FilterProfileID != nil {
		profileRepo := filterProfileRepository.NewFilterProfileRepository(db)
		profileModel, patterns, err := profileRepo.GetByID(*settings.FilterProfileID)
		if err != nil {
			return nil, err
		}
		base := ToProfile(profileModel, patterns)
		profile.ID = base.ID
		profile.Name = base.Name
		profile.Search = base.Search
		profile.Required = base.Required
		profile.Rejected = base.Rejected
		profile.Preferred = base.Preferred
	}

	profile.Search = dedupeStrings(append(profile.Search, settings.SearchPatterns...))

	if len(profile.Search) == 0 &&
		len(profile.Required) == 0 &&
		len(profile.Rejected) == 0 &&
		len(profile.Preferred) == 0 {
		return nil, nil
	}

	return profile, nil
}

func ToProfile(p filterProfileModel.FilterProfile, patterns []filterProfileModel.FilterPattern) *filter.Profile {
	profile := &filter.Profile{
		ID:   p.ID,
		Name: p.Name,
	}

	for _, pattern := range patterns {
		switch pattern.Kind {
		case filterProfileModel.KindSearch:
			profile.Search = append(profile.Search, pattern.Pattern)
		case filterProfileModel.KindRequired:
			profile.Required = append(profile.Required, pattern.Pattern)
		case filterProfileModel.KindRejected:
			profile.Rejected = append(profile.Rejected, pattern.Pattern)
		case filterProfileModel.KindPreferred:
			profile.Preferred = append(profile.Preferred, filter.PreferredPattern{
				Pattern: pattern.Pattern,
				Score:   pattern.Score,
			})
		}
	}

	return profile
}

func BuildContext(db *sqlx.DB, show showModel.Show, season, episode int, settings EffectiveSettings) (filter.Context, error) {
	showType := settings.ShowType
	if showType == "" {
		showType = showSettingsModel.ShowTypeStandard
	}

	ctx := filter.Context{
		Show:     utils.NormalizeTitle(show.Name),
		Season:   season,
		Episode:  episode,
		ShowType: showType,
	}

	if !settings.UseAliases {
		return ctx, nil
	}

	aliasesRepo := showAliasRepository.NewShowAliasesRepository(db)
	aliases, err := aliasesRepo.ListByShowID(show.ID)
	if err != nil {
		return filter.Context{}, err
	}

	seen := make(map[string]struct{})
	for _, alias := range aliases {
		if settings.OnlyLatin && !latinRegex.MatchString(alias.Alias) {
			continue
		}

		normalized := utils.NormalizeTitle(alias.Alias)
		if normalized == "" || normalized == ctx.Show {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}

		seen[normalized] = struct{}{}
		ctx.Aliases = append(ctx.Aliases, normalized)
	}

	return ctx, nil
}

// SearchPatterns returns the search patterns to use for a profile and show type.
// If showType is "anime", anime-specific defaults ({alias} - {episode:00}, etc.)
// are prepended; otherwise the standard S{season:00}E{episode:00} pattern is prepended.
func SearchPatterns(profile *filter.Profile, showType ...string) []string {
	var patterns []string
	if profile != nil {
		patterns = profile.Search
	}

	st := showSettingsModel.ShowTypeStandard
	if len(showType) > 0 && showType[0] != "" {
		st = showType[0]
	}

	if st == showSettingsModel.ShowTypeAnime {
		defaults := []string{
			filter.DefaultAnimeSearchPattern,
			filter.DefaultAnimeSeasonSearchPattern,
			filter.DefaultSearchPattern,
		}
		var missing []string
		for _, d := range defaults {
			found := false
			for _, p := range patterns {
				if p == d {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, d)
			}
		}
		return append(missing, patterns...)
	}

	for _, pattern := range patterns {
		if pattern == filter.DefaultSearchPattern {
			return patterns
		}
	}
	return append([]string{filter.DefaultSearchPattern}, patterns...)
}

// dedupeStrings removes duplicate strings while preserving first occurrence order.
func dedupeStrings(in []string) []string {
	if len(in) <= 1 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
