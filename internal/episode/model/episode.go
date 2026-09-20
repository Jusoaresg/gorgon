package model

import (
	"time"
)

const (
	TrackingWanted     string = "wanted"
	TrackingMissing    string = "missing"
	TrackingSkipped    string = "skipped"
	TrackingSnatched   string = "snatched"
	TrackingDownloaded string = "downloaded"
)

type Episode struct {
	ID       int64 `db:"id"`
	ShowID   int64 `db:"show_id"`
	SeasonID int64 `db:"season_id"`

	Name     string `db:"name"`
	Summary  string `db:"summary"`
	Type     string `db:"type"`
	Number   int    `db:"number"`
	Season   int    `db:"season"`
	AirStamp int64  `db:"airstamp"`

	Tracking string `db:"tracking"`
}

func (e *Episode) Create(
	ShowID int64,
	Name string,
	Summary string,
	Type string,
	Number int,
	Season int,
	AirStamp int64,
) *Episode {
	return &Episode{
		ShowID:   ShowID,
		Name:     Name,
		Summary:  Summary,
		Type:     Type,
		Number:   Number,
		Season:   Season,
		AirStamp: AirStamp,
	}
}

func (e *Episode) AirDateIn(loc *time.Location) string {
	if e.AirStamp == 0 {
		return ""
	}
	if loc == nil {
		loc = time.Local
	}
	return time.Unix(e.AirStamp, 0).In(loc).Format("2006-01-02")
}

func (e *Episode) AirDate() string {
	return e.AirDateIn(time.Local)
}

func (e *Episode) AirTimeIn(loc *time.Location) string {
	if e.AirStamp == 0 {
		return ""
	}
	if loc == nil {
		loc = time.Local
	}
	return time.Unix(e.AirStamp, 0).In(loc).Format("15:04")
}

func (e *Episode) AirTime() string {
	return e.AirTimeIn(time.Local)
}

func (e *Episode) HasAired() bool {
	if e.AirStamp == 0 {
		return false
	}
	return e.AirStamp <= time.Now().Unix()
}

func (e *Episode) SetNotInstalled() {
	e.Tracking = TrackingSkipped
}
