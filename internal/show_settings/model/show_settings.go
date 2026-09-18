package model

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

