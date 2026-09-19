package schema

import (
	"fmt"
	"html"
)

type EpisodeDownloadedTemplateInput struct {
	ShowName     string
	ShowURL      string
	Season       int
	Episode      int
	EpisodeTitle string
}

func FormatEpisodeDownloaded(input EpisodeDownloadedTemplateInput) string {
	showName := html.EscapeString(input.ShowName)
	epTitle := html.EscapeString(input.EpisodeTitle)
	epCode := fmt.Sprintf("S%02dE%02d", input.Season, input.Episode)

	showDisplay := showName
	if input.ShowURL != "" {
		showDisplay = fmt.Sprintf("<a href=\"%s\">%s</a>", html.EscapeString(input.ShowURL), showName)
	}

	header := fmt.Sprintf("✅ <b>Downloaded: %s</b>", showDisplay)
	if showName == "" {
		header = fmt.Sprintf("✅ <b>Downloaded: %s</b>", epCode)
	}

	type branch struct {
		label string
		value string
	}

	branches := []branch{
		{label: "Episode", value: epCode},
	}
	if epTitle != "" {
		branches = append(branches, branch{label: "Title", value: epTitle})
	}

	msg := header
	for i, b := range branches {
		prefix := "  ├"
		if i == len(branches)-1 {
			prefix = "  └"
		}
		msg += fmt.Sprintf("\n%s %s: %s", prefix, b.label, b.value)
	}

	msg += "\n\n🍿 <i>Ready to watch!</i>"

	return msg
}
