package schema

import (
	"fmt"
	"strings"
)

type DailySummaryEpisode struct {
	ShowName    string
	Season      int
	Number      int
	EpisodeName string
	AirTime     string
	Tracking    string
}

type DailySummaryTemplateInput struct {
	Date     string
	Episodes []DailySummaryEpisode
}

func FormatDailySummary(input DailySummaryTemplateInput) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📅 <b>Today's Releases (%s)</b>\n\n", input.Date))

	if len(input.Episodes) == 0 {
		sb.WriteString("<i>No releases scheduled for today.</i>")
		return sb.String()
	}

	for i, ep := range input.Episodes {
		epCode := fmt.Sprintf("S%02dE%02d", ep.Season, ep.Number)

		statusText := "⏳ Pending"
		switch strings.ToLower(ep.Tracking) {
		case "downloaded":
			statusText = "✅ Downloaded"
		case "snatched":
			statusText = "📥 Snatched"
		case "missing":
			statusText = "🔍 Missing"
		case "wanted":
			statusText = "⏳ Wanted"
		}

		sb.WriteString(fmt.Sprintf("🎬 <b>%s</b>\n", ep.ShowName))

		type branchItem struct {
			label string
			value string
		}

		branches := []branchItem{
			{label: "Episode", value: epCode},
		}

		if ep.AirTime != "" {
			branches = append(branches, branchItem{label: "Air Time", value: fmt.Sprintf("<code>%s</code>", ep.AirTime)})
		}

		branches = append(branches, branchItem{label: "Status", value: statusText})

		if ep.EpisodeName != "" {
			branches = append(branches, branchItem{label: "Title", value: fmt.Sprintf("<i>%s</i>", ep.EpisodeName)})
		}

		for bIdx, b := range branches {
			prefix := "├"
			if bIdx == len(branches)-1 {
				prefix = "└"
			}
			sb.WriteString(fmt.Sprintf("  %s <b>%s:</b> %s\n", prefix, b.label, b.value))
		}

		if i < len(input.Episodes)-1 {
			sb.WriteString("\n")
		}
	}

	if len(input.Episodes) == 1 {
		sb.WriteString("\n<i>Total: 1 episode scheduled for today.</i>")
	} else {
		sb.WriteString(fmt.Sprintf("\n<i>Total: %d episodes scheduled for today.</i>", len(input.Episodes)))
	}

	return sb.String()
}
