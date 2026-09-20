package service

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/telegram/schema"
)

type dailyEpisodeRow struct {
	ID       int64  `db:"id"`
	ShowID   int64  `db:"show_id"`
	ShowName string `db:"show_name"`
	Name     string `db:"name"`
	Number   int    `db:"number"`
	Season   int    `db:"season"`
	AirStamp int64  `db:"airstamp"`
	Tracking string `db:"tracking"`
}

func GetDailySummary(db *sqlx.DB, now time.Time) (schema.DailySummaryTemplateInput, error) {
	loc := config.GetAppLocation()
	if loc == nil {
		loc = time.Local
	}
	now = now.In(loc)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.AddDate(0, 0, 1)

	var rows []dailyEpisodeRow
	query := `
		SELECT e.id, e.show_id, e.name, e.number, e.season, e.airstamp, e.tracking,
		       s.name AS show_name
		FROM episodes e
		JOIN shows s ON e.show_id = s.id
		WHERE e.airstamp >= ? AND e.airstamp < ?
		  AND e.tracking != 'skipped'
		ORDER BY s.name ASC, e.season ASC, e.number ASC
	`

	if err := db.Select(&rows, query, dayStart.Unix(), dayEnd.Unix()); err != nil {
		return schema.DailySummaryTemplateInput{}, err
	}

	summaryEpisodes := make([]schema.DailySummaryEpisode, 0, len(rows))
	for _, row := range rows {
		airTimeStr := ""
		if row.AirStamp != 0 {
			airTimeStr = time.Unix(row.AirStamp, 0).In(loc).Format("15:04")
		}

		summaryEpisodes = append(summaryEpisodes, schema.DailySummaryEpisode{
			ShowName:    row.ShowName,
			Season:      row.Season,
			Number:      row.Number,
			EpisodeName: row.Name,
			AirTime:     airTimeStr,
			Tracking:    row.Tracking,
		})
	}

	return schema.DailySummaryTemplateInput{
		Date:     now.Format("02/01"),
		Episodes: summaryEpisodes,
	}, nil
}

func GenerateDailySummaryText(db *sqlx.DB, now time.Time) string {
	input, err := GetDailySummary(db, now)
	if err != nil {
		return fmt.Sprintf("⚠️ Failed to load schedule: %s", err.Error())
	}
	return schema.FormatDailySummary(input)
}
