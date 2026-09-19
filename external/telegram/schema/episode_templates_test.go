package schema

import (
	"strings"
	"testing"
)

func TestFormatEpisodeDownloaded(t *testing.T) {
	input := EpisodeDownloadedTemplateInput{
		ShowName:     "Breaking Bad",
		ShowURL:      "https://www.tvmaze.com/shows/169/breaking-bad",
		Season:       5,
		Episode:      16,
		EpisodeTitle: "Felina",
	}

	result := FormatEpisodeDownloaded(input)

	for _, want := range []string{
		"✅ <b>Downloaded: <a href=\"https://www.tvmaze.com/shows/169/breaking-bad\">Breaking Bad</a></b>",
		"├ Episode: S05E16",
		"└ Title: Felina",
		"🍿 <i>Ready to watch!</i>",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("FormatEpisodeDownloaded missing %q in result:\n%s", want, result)
		}
	}
}

func TestFormatEpisodeSnatched(t *testing.T) {
	input := EpisodeSnatchedTemplateInput{
		ShowName:     "Severance",
		Season:       2,
		Episode:      1,
		EpisodeTitle: "Hello Ms. Cobel",
		ReleaseTitle: "Severance.S02E01.1080p.WEB-DL",
		ReleaseURL:   "https://indexer.example/torrent/123",
		Indexer:      "TorrentLeech",
	}

	result := FormatEpisodeSnatched(input)

	for _, want := range []string{
		"📥 <b>Snatched: Severance</b>",
		"├ Episode: S02E01",
		"├ Indexer: <code>TorrentLeech</code>",
		"└ Title: Hello Ms. Cobel",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("FormatEpisodeSnatched missing %q in result:\n%s", want, result)
		}
	}
}
