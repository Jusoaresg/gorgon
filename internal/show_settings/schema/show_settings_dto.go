package schema

import (
	showSettingsModel "github.com/jusoaresg/gorgon/internal/show_settings/model"
)

type ShowSettingsDto struct {
	FilterProfileID *int64   `json:"filter_profile_id"`
	ShowType        string   `json:"show_type"`
	UseAliases      bool     `json:"use_aliases"`
	OnlyLatin       bool     `json:"only_latin"`
	SearchPatterns  []string `json:"search_patterns"`
}

func ToShowSettingsDto(settings showSettingsModel.ShowSettings, searchPatterns []string) ShowSettingsDto {
	if searchPatterns == nil {
		searchPatterns = []string{}
	}
	showType := settings.ShowType
	if showType == "" {
		showType = showSettingsModel.ShowTypeStandard
	}
	return ShowSettingsDto{
		FilterProfileID: settings.FilterProfileID,
		ShowType:        showType,
		UseAliases:      settings.UseAliases,
		OnlyLatin:       settings.OnlyLatin,
		SearchPatterns:  searchPatterns,
	}
}

