package schema

import (
	filterProfileModel "github.com/jusoaresg/gorgon/internal/filter_profile/model"
)

type FilterPatternDto struct {
	Kind    string `json:"kind"`
	Pattern string `json:"pattern"`
	Score   int    `json:"score"`
}

type SaveFilterProfileRequest struct {
	Name              string             `json:"name"`
	UpgradeUntilScore int                `json:"upgrade_until_score"`
	Patterns          []FilterPatternDto `json:"patterns"`
}

type FilterProfileDto struct {
	ID                int64              `json:"id"`
	Name              string             `json:"name"`
	UpgradeUntilScore int                `json:"upgrade_until_score"`
	Patterns          []FilterPatternDto `json:"patterns"`
}

type FilterProfileListDto struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	UpgradeUntilScore int    `json:"upgrade_until_score"`
}

func ToFilterPatterns(requests []FilterPatternDto) []filterProfileModel.FilterPattern {
	patterns := make([]filterProfileModel.FilterPattern, 0, len(requests))
	for _, request := range requests {
		patterns = append(patterns, filterProfileModel.FilterPattern{
			Kind:    filterProfileModel.FilterPatternKind(request.Kind),
			Pattern: request.Pattern,
			Score:   request.Score,
		})
	}
	return patterns
}

func ToProfileDto(profile filterProfileModel.FilterProfile, patterns []filterProfileModel.FilterPattern) FilterProfileDto {
	dto := FilterProfileDto{
		ID:                profile.ID,
		Name:              profile.Name,
		UpgradeUntilScore: profile.UpgradeUntilScore,
		Patterns:          make([]FilterPatternDto, 0, len(patterns)),
	}
	for _, pattern := range patterns {
		dto.Patterns = append(dto.Patterns, FilterPatternDto{
			Kind:    string(pattern.Kind),
			Pattern: pattern.Pattern,
			Score:   pattern.Score,
		})
	}
	return dto
}

func ToProfileListDto(profiles []filterProfileModel.FilterProfile) []FilterProfileListDto {
	list := make([]FilterProfileListDto, 0, len(profiles))
	for _, profile := range profiles {
		list = append(list, FilterProfileListDto{
			ID:                profile.ID,
			Name:              profile.Name,
			UpgradeUntilScore: profile.UpgradeUntilScore,
		})
	}
	return list
}
