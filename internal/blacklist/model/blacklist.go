package model

type ReleaseBlacklist struct {
	ID           int64  `db:"id" json:"id"`
	ShowID       int64  `db:"show_id" json:"show_id"`
	EpisodeID    *int64 `db:"episode_id" json:"episode_id,omitempty"`
	InfoHash     string `db:"info_hash" json:"info_hash"`
	ReleaseTitle string `db:"release_title" json:"release_title"`
	Indexer      string `db:"indexer" json:"indexer"`
	Reason       string `db:"reason" json:"reason"`
	CreatedAt    int64  `db:"created_at" json:"created_at"`
	ExpiresAt    *int64 `db:"expires_at" json:"expires_at,omitempty"`
}
