package model

import "strings"

const (
	ShowTypeStandard = "standard"
	ShowTypeAnime    = "anime"
)

type ShowSettings struct {
	ShowID          int64  `db:"show_id"`
	FilterProfileID *int64 `db:"filter_profile_id"`
	ShowType        string `db:"show_type"`
	UseAliases      bool   `db:"use_aliases"`
	OnlyLatin       bool   `db:"only_latin"`
	CreatedAt       int64  `db:"created_at"`
	UpdatedAt       int64  `db:"updated_at"`
}

// DetectShowType determines if a show should be categorized as anime or standard
// based on its genres and/or raw show type.
func DetectShowType(genres []string, rawType ...string) string {
	for _, t := range rawType {
		if strings.EqualFold(strings.TrimSpace(t), "anime") {
			return ShowTypeAnime
		}
	}
	for _, g := range genres {
		for _, part := range strings.Split(g, ",") {
			if strings.EqualFold(strings.TrimSpace(part), "anime") {
				return ShowTypeAnime
			}
		}
	}
	return ShowTypeStandard
}

