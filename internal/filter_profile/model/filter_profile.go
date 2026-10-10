package model

type FilterPatternKind string

const (
	KindSearch    FilterPatternKind = "search"
	KindRequired  FilterPatternKind = "required"
	KindRejected  FilterPatternKind = "rejected"
	KindPreferred FilterPatternKind = "preferred"
)

type FilterProfile struct {
	ID                int64  `db:"id"`
	Name              string `db:"name"`
	UpgradeUntilScore int    `db:"upgrade_until_score"`
	CreatedAt         int64  `db:"created_at"`
	UpdatedAt         int64  `db:"updated_at"`
}

type FilterPattern struct {
	ID        int64             `db:"id"`
	ProfileID int64             `db:"profile_id"`
	Kind      FilterPatternKind `db:"kind"`
	Pattern   string            `db:"pattern"`
	Score     int               `db:"score"`
	Position  int               `db:"position"`
}
