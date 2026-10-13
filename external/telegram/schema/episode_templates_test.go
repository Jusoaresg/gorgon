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

func TestFormatEpisodeSnatched_Upgrade(t *testing.T) {
	input := EpisodeSnatchedTemplateInput{
		ShowName:       "Frieren",
		Season:         1,
		Episode:        5,
		EpisodeTitle:   "Phantom of the Dead",
		ReleaseTitle:   "[SubsPlease] Frieren - 05 (1080p).mkv",
		OldReleaseName: "[Erai-raws] Frieren - 05 (720p).mkv",
		ReleaseURL:     "https://nyaa.si/view/123",
		Indexer:        "Nyaa",
		Score:          70,
		IsUpgrade:      true,
		OldScore:       20,
		CutoffScore:    60,
	}

	result := FormatEpisodeSnatched(input)

	for _, want := range []string{
		"⬆️ <b>Upgrade Snatched: Frieren</b>",
		"├ Episode: S01E05",
		"├ Score: <code>20 → 70</code>",
		"├ Cutoff: <code>70 / 60</code> (Cutoff Reached 🎯)",
		"├ Release: <code>[SubsPlease] Frieren - 05 (1080p).mkv</code>",
		"├ Replaced: <code>[Erai-raws] Frieren - 05 (720p).mkv</code>",
		"├ Indexer: <code>Nyaa</code>",
		"└ Title: Phantom of the Dead",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("FormatEpisodeSnatched upgrade missing %q in result:\n%s", want, result)
		}
	}
}

func TestFormatReleaseBlacklisted(t *testing.T) {
	input := ReleaseBlacklistedTemplateInput{
		ShowName:     "Dandadan",
		Season:       1,
		Episode:      3,
		EpisodeTitle: "Granny vs Granny",
		ReleaseTitle: "[SubsPlease] Dandadan - 03 (1080p).mkv",
		Indexer:      "Nyaa",
		Reason:       "stalled download timeout (stalledDL)",
	}

	result := FormatReleaseBlacklisted(input)

	for _, want := range []string{
		"🚫 <b>Blacklisted: Dandadan</b>",
		"├ Episode: S01E03",
		"├ Indexer: <code>Nyaa</code>",
		"├ Reason: <code>stalled download timeout (stalledDL)</code>",
		"└ Title: Granny vs Granny",
		"🔄 <i>Searching for another release...</i>",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("FormatReleaseBlacklisted missing %q in result:\n%s", want, result)
		}
	}
}
