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
	IsUpgrade    bool
	Score        int
	ReleaseTitle string
}

func FormatEpisodeDownloaded(input EpisodeDownloadedTemplateInput) string {
	showName := html.EscapeString(input.ShowName)
	epTitle := html.EscapeString(input.EpisodeTitle)
	releaseTitle := html.EscapeString(input.ReleaseTitle)
	epCode := fmt.Sprintf("S%02dE%02d", input.Season, input.Episode)

	showDisplay := showName
	if input.ShowURL != "" {
		showDisplay = fmt.Sprintf("<a href=\"%s\">%s</a>", html.EscapeString(input.ShowURL), showName)
	}

	headerPrefix := "✅ <b>Downloaded:"
	if input.IsUpgrade {
		headerPrefix = "⬆️ <b>Upgraded:"
	}

	header := fmt.Sprintf("%s %s</b>", headerPrefix, showDisplay)
	if showName == "" {
		header = fmt.Sprintf("%s %s</b>", headerPrefix, epCode)
	}

	type branch struct {
		label string
		value string
	}

	branches := []branch{
		{label: "Episode", value: epCode},
	}
	if input.IsUpgrade && input.Score > 0 {
		branches = append(branches, branch{label: "Score", value: fmt.Sprintf("<code>+%d</code>", input.Score)})
	}
	if input.IsUpgrade && releaseTitle != "" {
		branches = append(branches, branch{label: "Release", value: fmt.Sprintf("<code>%s</code>", releaseTitle)})
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
