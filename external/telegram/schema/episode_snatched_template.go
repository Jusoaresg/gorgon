package schema

import (
	"fmt"
	"html"
)

type EpisodeSnatchedTemplateInput struct {
	EpisodeID      int64
	ShowName       string
	ShowURL        string
	Season         int
	Episode        int
	EpisodeTitle   string
	ReleaseTitle   string
	ReleaseURL     string
	Indexer        string
	Score          int
	IsUpgrade      bool
	OldScore       int
	CutoffScore    int
	OldReleaseName string
}

func FormatEpisodeSnatched(input EpisodeSnatchedTemplateInput) string {
	showName := html.EscapeString(input.ShowName)
	epTitle := html.EscapeString(input.EpisodeTitle)
	indexer := html.EscapeString(input.Indexer)
	releaseTitle := html.EscapeString(input.ReleaseTitle)
	oldRelease := html.EscapeString(input.OldReleaseName)
	epCode := fmt.Sprintf("S%02dE%02d", input.Season, input.Episode)

	showDisplay := showName
	if input.ShowURL != "" {
		showDisplay = fmt.Sprintf("<a href=\"%s\">%s</a>", html.EscapeString(input.ShowURL), showName)
	}

	headerPrefix := "📥 <b>Snatched:"
	if input.IsUpgrade {
		headerPrefix = "⬆️ <b>Upgrade Snatched:"
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

	if input.IsUpgrade {
		branches = append(branches, branch{
			label: "Score",
			value: fmt.Sprintf("<code>%d → %d</code>", input.OldScore, input.Score),
		})
		if input.CutoffScore > 0 {
			cutoffLabel := fmt.Sprintf("<code>%d / %d</code>", input.Score, input.CutoffScore)
			if input.Score >= input.CutoffScore {
				cutoffLabel += " (Cutoff Reached 🎯)"
			}
			branches = append(branches, branch{
				label: "Cutoff",
				value: cutoffLabel,
			})
		}
		if releaseTitle != "" {
			branches = append(branches, branch{
				label: "Release",
				value: fmt.Sprintf("<code>%s</code>", releaseTitle),
			})
		}
		if oldRelease != "" {
			branches = append(branches, branch{
				label: "Replaced",
				value: fmt.Sprintf("<code>%s</code>", oldRelease),
			})
		}
	} else if input.Score > 0 {
		branches = append(branches, branch{
			label: "Score",
			value: fmt.Sprintf("<code>+%d</code>", input.Score),
		})
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

type ReleaseBlacklistedTemplateInput struct {
	ShowName     string
	Season       int
	Episode      int
	EpisodeTitle string
	ReleaseTitle string
	ReleaseURL   string
	Indexer      string
	Reason       string
}

func FormatReleaseBlacklisted(input ReleaseBlacklistedTemplateInput) string {
	showName := html.EscapeString(input.ShowName)
	epTitle := html.EscapeString(input.EpisodeTitle)
	releaseTitle := html.EscapeString(input.ReleaseTitle)
	indexer := html.EscapeString(input.Indexer)
	reason := html.EscapeString(input.Reason)
	epCode := fmt.Sprintf("S%02dE%02d", input.Season, input.Episode)

	header := fmt.Sprintf("🚫 <b>Blacklisted: %s</b>", showName)
	if showName == "" {
		header = fmt.Sprintf("🚫 <b>Blacklisted: %s</b>", epCode)
	}

	type branch struct {
		label string
		value string
	}

	var branches []branch
	if input.Season > 0 || input.Episode > 0 {
		branches = append(branches, branch{label: "Episode", value: epCode})
	}
	if releaseTitle != "" {
		branches = append(branches, branch{label: "Release", value: fmt.Sprintf("<code>%s</code>", releaseTitle)})
	}
	if indexer != "" {
		branches = append(branches, branch{label: "Indexer", value: fmt.Sprintf("<code>%s</code>", indexer)})
	}
	if reason != "" {
		branches = append(branches, branch{label: "Reason", value: fmt.Sprintf("<code>%s</code>", reason)})
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

	msg += "\n\n🔄 <i>Searching for another release...</i>"
	return msg
}
