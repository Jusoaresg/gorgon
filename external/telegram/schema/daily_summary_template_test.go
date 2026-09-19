package schema

import (
	"strings"
	"testing"
)

func TestFormatDailySummary_WithEpisodes(t *testing.T) {
	input := DailySummaryTemplateInput{
		Date: "19/09",
		Episodes: []DailySummaryEpisode{
			{
				ShowName:    "Frieren: Beyond Journey's End",
				Season:      1,
				Number:      5,
				EpisodeName: "Phantom of the Dead",
				AirTime:     "11:00",
				Tracking:    "downloaded",
			},
			{
				ShowName:    "Dandadan",
				Season:      1,
				Number:      2,
				EpisodeName: "That's a Space Alien, Ain't It?",
				AirTime:     "13:00",
				Tracking:    "wanted",
			},
		},
	}

	res := FormatDailySummary(input)

	if !strings.Contains(res, "📅 <b>Today's Releases (19/09)</b>") {
		t.Errorf("expected header with date, got %s", res)
	}

	if !strings.Contains(res, "🎬 <b>Frieren: Beyond Journey's End</b>") {
		t.Errorf("expected Frieren header, got %s", res)
	}

	if !strings.Contains(res, "  ├ <b>Episode:</b> S01E05") {
		t.Errorf("expected Frieren episode line, got %s", res)
	}

	if !strings.Contains(res, "  ├ <b>Air Time:</b> <code>11:00</code>") {
		t.Errorf("expected Frieren air time line, got %s", res)
	}

	if !strings.Contains(res, "  ├ <b>Status:</b> ✅ Downloaded") {
		t.Errorf("expected Frieren status line, got %s", res)
	}

	if !strings.Contains(res, "  └ <b>Title:</b> <i>Phantom of the Dead</i>") {
		t.Errorf("expected Frieren title line as tree end, got %s", res)
	}

	if !strings.Contains(res, "🎬 <b>Dandadan</b>") {
		t.Errorf("expected Dandadan header, got %s", res)
	}

	if !strings.Contains(res, "  ├ <b>Episode:</b> S01E02") {
		t.Errorf("expected Dandadan episode line, got %s", res)
	}

	if !strings.Contains(res, "  ├ <b>Air Time:</b> <code>13:00</code>") {
		t.Errorf("expected Dandadan air time line, got %s", res)
	}

	if !strings.Contains(res, "  ├ <b>Status:</b> ⏳ Wanted") {
		t.Errorf("expected Dandadan status line, got %s", res)
	}

	if !strings.Contains(res, "  └ <b>Title:</b> <i>That's a Space Alien, Ain't It?</i>") {
		t.Errorf("expected Dandadan title line as tree end, got %s", res)
	}

	if !strings.Contains(res, "<i>Total: 2 episodes scheduled for today.</i>") {
		t.Errorf("expected total 2 episodes footer, got %s", res)
	}
}

func TestFormatDailySummary_SingleEpisode(t *testing.T) {
	input := DailySummaryTemplateInput{
		Date: "20/09",
		Episodes: []DailySummaryEpisode{
			{
				ShowName: "Solo Leveling",
				Season:   2,
				Number:   1,
				AirTime:  "15:30",
				Tracking: "snatched",
			},
		},
	}

	res := FormatDailySummary(input)

	if !strings.Contains(res, "🎬 <b>Solo Leveling</b>") {
		t.Errorf("expected Solo Leveling header, got %s", res)
	}

	if !strings.Contains(res, "  ├ <b>Episode:</b> S02E01") {
		t.Errorf("expected Solo Leveling episode line, got %s", res)
	}

	if !strings.Contains(res, "  ├ <b>Air Time:</b> <code>15:30</code>") {
		t.Errorf("expected Solo Leveling air time line, got %s", res)
	}

	if !strings.Contains(res, "  └ <b>Status:</b> 📥 Snatched") {
		t.Errorf("expected Solo Leveling status line as tree end when no title, got %s", res)
	}

	if !strings.Contains(res, "<i>Total: 1 episode scheduled for today.</i>") {
		t.Errorf("expected singular total 1 episode footer, got %s", res)
	}
}

func TestFormatDailySummary_Empty(t *testing.T) {
	input := DailySummaryTemplateInput{
		Date:     "21/09",
		Episodes: []DailySummaryEpisode{},
	}

	res := FormatDailySummary(input)

	if !strings.Contains(res, "<i>No releases scheduled for today.</i>") {
		t.Errorf("expected empty message, got %s", res)
	}
}
