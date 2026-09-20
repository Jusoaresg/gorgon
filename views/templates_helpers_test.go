package views

import (
	"testing"
	"time"

	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	"github.com/stretchr/testify/assert"
)

func TestFormatNextEpisodeLabelAndInfo(t *testing.T) {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		loc = time.FixedZone("BRT", -3*60*60)
	}

	// Reference time: Saturday, 2026-09-19 at 20:00 BRT
	now := time.Date(2026, 9, 19, 20, 0, 0, 0, loc)

	t.Run("ended show returns Ended", func(t *testing.T) {
		label := formatNextEpisodeLabelAt(nil, "Ended", now, loc)
		assert.Equal(t, "Ended", label)

		info := formatNextEpisodeInfoAt(nil, "Ended", now, loc)
		assert.Equal(t, "Ended", info)
	})

	t.Run("no upcoming episodes returns TBD", func(t *testing.T) {
		pastEp := episodeModel.Episode{
			Number:   1,
			Season:   1,
			AirStamp: now.Add(-2 * time.Hour).Unix(),
		}
		label := formatNextEpisodeLabelAt([]episodeModel.Episode{pastEp}, "Running", now, loc)
		assert.Equal(t, "TBD", label)

		info := formatNextEpisodeInfoAt([]episodeModel.Episode{pastEp}, "Running", now, loc)
		assert.Equal(t, "TBD", info)
	})

	t.Run("episode airing later today returns Today", func(t *testing.T) {
		todayEp := episodeModel.Episode{
			Number:   1,
			Season:   1,
			AirStamp: time.Date(2026, 9, 19, 23, 30, 0, 0, loc).Unix(),
		}
		label := formatNextEpisodeLabelAt([]episodeModel.Episode{todayEp}, "Running", now, loc)
		assert.Equal(t, "Today", label)

		info := formatNextEpisodeInfoAt([]episodeModel.Episode{todayEp}, "Running", now, loc)
		assert.Equal(t, "S01E01 • Today", info)
	})

	t.Run("episode airing early tomorrow (Sunday 02:00) returns Tomorrow", func(t *testing.T) {
		// Only 6 hours away from Saturday 20:00, but is calendar tomorrow
		earlyTomorrowEp := episodeModel.Episode{
			Number:   2,
			Season:   1,
			AirStamp: time.Date(2026, 9, 20, 2, 0, 0, 0, loc).Unix(),
		}
		label := formatNextEpisodeLabelAt([]episodeModel.Episode{earlyTomorrowEp}, "Running", now, loc)
		assert.Equal(t, "Tomorrow", label)

		info := formatNextEpisodeInfoAt([]episodeModel.Episode{earlyTomorrowEp}, "Running", now, loc)
		assert.Equal(t, "S01E02 • Tomorrow", info)
	})

	t.Run("episode airing tomorrow evening (Sunday 21:00) returns Tomorrow", func(t *testing.T) {
		tomorrowEp := episodeModel.Episode{
			Number:   3,
			Season:   1,
			AirStamp: time.Date(2026, 9, 20, 21, 0, 0, 0, loc).Unix(),
		}
		label := formatNextEpisodeLabelAt([]episodeModel.Episode{tomorrowEp}, "Running", now, loc)
		assert.Equal(t, "Tomorrow", label)

		info := formatNextEpisodeInfoAt([]episodeModel.Episode{tomorrowEp}, "Running", now, loc)
		assert.Equal(t, "S01E03 • Tomorrow", info)
	})

	t.Run("CRITICAL: episode airing Monday 14:00 (42h away) returns Mon and NEVER Tomorrow", func(t *testing.T) {
		// Saturday 20:00 to Monday 14:00 is 42 hours.
		// The old buggy code did: int(42/24) = 1, showing "Tomorrow" for Monday!
		mondayEp := episodeModel.Episode{
			Number:   4,
			Season:   1,
			AirStamp: time.Date(2026, 9, 21, 14, 0, 0, 0, loc).Unix(),
		}
		label := formatNextEpisodeLabelAt([]episodeModel.Episode{mondayEp}, "Running", now, loc)
		assert.Equal(t, "Mon", label)

		info := formatNextEpisodeInfoAt([]episodeModel.Episode{mondayEp}, "Running", now, loc)
		assert.Equal(t, "S01E04 • Mon", info)
	})

	t.Run("episode airing in 5 days returns weekday name", func(t *testing.T) {
		thursdayEp := episodeModel.Episode{
			Number:   5,
			Season:   1,
			AirStamp: time.Date(2026, 9, 24, 18, 0, 0, 0, loc).Unix(),
		}
		label := formatNextEpisodeLabelAt([]episodeModel.Episode{thursdayEp}, "Running", now, loc)
		assert.Equal(t, "Thu", label)

		info := formatNextEpisodeInfoAt([]episodeModel.Episode{thursdayEp}, "Running", now, loc)
		assert.Equal(t, "S01E05 • Thu", info)
	})

	t.Run("episode airing in 10 days returns Month Day", func(t *testing.T) {
		laterEp := episodeModel.Episode{
			Number:   6,
			Season:   1,
			AirStamp: time.Date(2026, 9, 29, 18, 0, 0, 0, loc).Unix(),
		}
		label := formatNextEpisodeLabelAt([]episodeModel.Episode{laterEp}, "Running", now, loc)
		assert.Equal(t, "Sep 29", label)

		info := formatNextEpisodeInfoAt([]episodeModel.Episode{laterEp}, "Running", now, loc)
		assert.Equal(t, "S01E06 • Sep 29", info)
	})
}
