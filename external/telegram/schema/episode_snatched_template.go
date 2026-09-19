package schema

import (
	"fmt"
	"html"
)

type EpisodeSnatchedTemplateInput struct {
	ShowName     string
	ShowURL      string
	Season       int
	Episode      int
	EpisodeTitle string
	ReleaseTitle string
	ReleaseURL   string
	Indexer      string
}

func FormatEpisodeSnatched(input EpisodeSnatchedTemplateInput) string {
	showName := html.EscapeString(input.ShowName)
	epTitle := html.EscapeString(input.EpisodeTitle)
	indexer := html.EscapeString(input.Indexer)
	epCode := fmt.Sprintf("S%02dE%02d", input.Season, input.Episode)

	showDisplay := showName
	if input.ShowURL != "" {
		showDisplay = fmt.Sprintf("<a href=\"%s\">%s</a>", html.EscapeString(input.ShowURL), showName)
	}

	header := fmt.Sprintf("📥 <b>Snatched: %s</b>", showDisplay)
	if showName == "" {
		header = fmt.Sprintf("📥 <b>Snatched: %s</b>", epCode)
	}

	type branch struct {
		label string
		value string
	}

	branches := []branch{
		{label: "Episode", value: epCode},
	}
	if indexer != "" {
		branches = append(branches, branch{label: "Indexer", value: fmt.Sprintf("<code>%s</code>", indexer)})
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

	return msg
}
