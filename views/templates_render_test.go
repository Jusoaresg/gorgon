package views

import (
	"bytes"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/jusoaresg/gorgon/external/qbittorrent/schema"
	"github.com/jusoaresg/gorgon/internal/downloads/service"
	filterProfileModel "github.com/jusoaresg/gorgon/internal/filter_profile/model"
	filterSettingsModel "github.com/jusoaresg/gorgon/internal/filter_settings/model"
	showModel "github.com/jusoaresg/gorgon/internal/show/model"
	showAliasModel "github.com/jusoaresg/gorgon/internal/show_aliases/model"
	showSettingsModel "github.com/jusoaresg/gorgon/internal/show_settings/model"
	"github.com/jusoaresg/gorgon/pkg/schemas"
)

type renderData struct {
	Items        []service.DownloadItem
	Summary      service.DownloadsSummary
	ErrorMessage string
}

func TestRenderDownloadsPage(t *testing.T) {
	tmpl := NewTemplate()

	items := []service.DownloadItem{
		{
			Torrent: schema.CheckTorrentResponse{
				Name:      "My.Show.S01E01.1080p.WEB-DL",
				Hash:      "abc123",
				State:     "downloading",
				Category:  "gorgon",
				Progress:  0.42,
				Size:      734003200,
				Completed: 308281344,
				DlSpeed:   5242880,
				UpSpeed:   1048576,
				NumSeeds:  12,
				NumLeechs: 3,
				Eta:       98,
			},
			Episode: &service.EpisodeInfo{
				EpisodeID: 1,
				ShowID:    1,
				Name:      "Pilot",
				Season:    1,
				Number:    1,
				ShowName:  "Breaking Test",
				ShowImage: "https://example.com/img.jpg",
				InfoUrl:   "https://example.com/release/1",
			},
		},
		{
			Torrent: schema.CheckTorrentResponse{
				Name:     "Some.Other.Torrent",
				Hash:     "xyz",
				State:    "stalledDL",
				Category: "gorgon",
				Progress: 0.0,
				Eta:      -1,
			},
		},
		{
			Torrent: schema.CheckTorrentResponse{
				Name:      "My.Show.S01E02.1080p.WEB-DL",
				Hash:      "donehash",
				State:     "uploading",
				Category:  "gorgon",
				Progress:  1.0,
				Size:      500000000,
				Completed: 500000000,
			},
			Episode: &service.EpisodeInfo{
				EpisodeID: 2,
				ShowID:    1,
				Name:      "Second Episode",
				Season:    1,
				Number:    2,
				Tracking:  "snatched",
				ShowName:  "Breaking Test",
			},
		},
	}

	var buf bytes.Buffer
	if err := tmpl.templates.ExecuteTemplate(&buf, "download-items", PageData{Data: renderData{Items: items}}); err != nil {
		t.Fatalf("failed to render download-items: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		"Breaking Test",
		"S01E01",
		"Pilot",
		"42%",
		"Downloading",
		"5.0 MB/s",
		"1.0 MB/s",
		"12 / 3",
		"1m 38s",
		"294.0 MB / 700.0 MB",
		"Some.Other.Torrent",
		"Stalled",
		"Waiting to Import",
		"100%",
		"hx-trigger=\"every 2s\"",
	} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("rendered output missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderDownloadsEmpty(t *testing.T) {
	tmpl := NewTemplate()

	var buf bytes.Buffer
	if err := tmpl.templates.ExecuteTemplate(&buf, "download-items", PageData{Data: renderData{}}); err != nil {
		t.Fatalf("failed to render empty download-items: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("No active downloads")) {
		t.Errorf("expected empty state, got:\n%s", buf.String())
	}
}

func TestRenderFilterSettings(t *testing.T) {
	tmpl := NewTemplate()

	profileID := int64(1)
	data := struct {
		FilterSettings filterSettingsModel.FilterSettings
		Profiles       []filterProfileModel.FilterProfile
	}{
		FilterSettings: filterSettingsModel.FilterSettings{
			DefaultFilterProfileID: &profileID,
			UseAliases:             true,
			OnlyLatin:              true,
		},
		Profiles: []filterProfileModel.FilterProfile{
			{ID: 1, Name: "HD"},
			{ID: 2, Name: "SD"},
		},
	}

	var buf bytes.Buffer
	if err := tmpl.templates.ExecuteTemplate(&buf, "filterSettings", data); err != nil {
		t.Fatalf("failed to render filterSettings: %v", err)
	}

	for _, want := range []string{
		"Default Filter Profile",
		"HD",
		"SD",
		"Save Profile",
		"only_latin",
		"json-enc-custom",
		"parse-types",
		"empty-as-null",
		"checkbox-box",
		"Placeholder reference",
		"{season:00}",
		"{absolute}",
	} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("rendered output missing %q", want)
		}
	}
}

func TestRenderTelegramSettings(t *testing.T) {
	tmpl := NewTemplate()

	data := schemas.ConfigFile{
		TelegramBotApiKey:           "test-token-123",
		TelegramChatID:              "987654321",
		TelegramDailySummaryEnabled: true,
		TelegramDailySummaryTime:    "08:00",
		TelegramNotifyEmptySummary:  false,
	}

	var buf bytes.Buffer
	if err := tmpl.templates.ExecuteTemplate(&buf, "telegramSettings", data); err != nil {
		t.Fatalf("failed to render telegramSettings: %v", err)
	}

	for _, want := range []string{
		"Telegram",
		"Bot API Token",
		"telegramBotApiKey",
		"test-token-123",
		"Chat ID",
		"telegramChatID",
		"987654321",
		"Auto-Detect",
		"/api/v1/telegram/detect-chat-id",
		"Daily Summary",
		"telegramDailySummaryEnabled",
		"telegramDailySummaryTime",
		"telegramNotifyEmptySummary",
		"Test Notification",
		"/summary",
		"/today",
		"Save Changes",
	} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("rendered output missing %q", want)
		}
	}
}

func TestRenderEditShowModal(t *testing.T) {
	tmpl := NewTemplate()

	profileID := int64(1)
	data := struct {
		Show           showModel.Show
		Profiles       []filterProfileModel.FilterProfile
		Settings       showSettingsModel.ShowSettings
		SearchPatterns []string
		Aliases        []showAliasModel.ShowAlias
	}{
		Show: showModel.Show{
			ID:   42,
			Name: "Dragon Ball",
		},
		Profiles: []filterProfileModel.FilterProfile{
			{ID: 1, Name: "HD"},
			{ID: 2, Name: "SD"},
		},
		Settings: showSettingsModel.ShowSettings{
			FilterProfileID: &profileID,
			UseAliases:      true,
			OnlyLatin:       true,
		},
		SearchPatterns: []string{
			"{alias} 4k",
			"{alias} 1080p",
		},
		Aliases: []showAliasModel.ShowAlias{
			{ID: 1, Alias: "DBZ", Source: "user"},
			{ID: 2, Alias: "ドラゴンボール", Source: "tvmaze"},
		},
	}

	var buf bytes.Buffer
	if err := tmpl.templates.ExecuteTemplate(&buf, "edit-show-modal", data); err != nil {
		t.Fatalf("failed to render edit-show-modal: %v", err)
	}

	for _, want := range []string{
		"Edit Series",
		"Dragon Ball",
		"modal-content modal-lg",
		"modal-layout",
		"modal-sidebar",
		"modal-section-item",
		"data-section=\"filters\"",
		"data-show-id=\"42\"",
		"filter_profile_id",
		"use_aliases",
		"only_latin",
		"Search Patterns",
		"show-search-pattern-rows",
		"addShowSearchPatternRow",
		"checkbox-box",
		"handleAliasAdded",
		"DBZ",
		"Delete",
		"ドラゴンボール",
		"synced",
		"Custom Aliases",
		"Delete Series",
		"Save Changes",
		"{alias} S{season:00}E{episode:00}",
		"{alias} 4k",
		"{alias} 1080p",
	} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("rendered output missing %q", want)
		}
	}
}

func TestRenderDownloadsFullLayout(t *testing.T) {
	tmpl := NewTemplate()

	var buf bytes.Buffer
	err := tmpl.templates.ExecuteTemplate(&buf, "layout", PageData{
		TemplateName: "downloads",
		Styles:       []string{"downloads.css"},
		Data: renderData{
			Items: []service.DownloadItem{
				{
					Torrent: schema.CheckTorrentResponse{
						Name:      "My.Show.S01E01.1080p.WEB-DL",
						Hash:      "abc123",
						State:     "downloading",
						Category:  "gorgon",
						Progress:  0.42,
						Size:      734003200,
						Completed: 308281344,
						DlSpeed:   5242880,
						NumSeeds:  12,
						NumLeechs: 3,
						Eta:       98,
					},
					Episode: &service.EpisodeInfo{
						EpisodeID: 1,
						ShowID:    1,
						Name:      "Pilot",
						Season:    1,
						Number:    1,
						ShowName:  "Breaking Test",
						ShowImage: "https://example.com/img.jpg",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to render full layout: %v", err)
	}

	for _, want := range []string{
		"href=\"/downloads\"",
		"class=\"sidebar-icon\"",
		"downloads.css",
		"Breaking Test",
		"S01E01",
		"42%",
		"hx-trigger=\"every 2s\"",
	} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("rendered layout missing %q", want)
		}
	}
}

// logsDataTest mirrors internal/show/web.LogsData (cannot be imported here as
// the web package already imports views).
type logsDataTest struct {
	Entries       []logsEntryTest
	CurrentFile   string
	Search        string
	Level         string
	Page          int
	PageSize      int
	TotalPages    int
	TotalEntries  int
	IsCurrentFile bool
}

func (d logsDataTest) BaseQuery() string {
	v := url.Values{}
	v.Set("view", "log-entries")
	v.Set("file", d.CurrentFile)
	if d.Level != "" {
		v.Set("level", d.Level)
	}
	if d.Search != "" {
		v.Set("search", d.Search)
	}
	return v.Encode()
}

func (d logsDataTest) PageURL(page int) string {
	return "/logs?" + d.BaseQuery() + "&page=" + strconv.Itoa(page) + "&pageSize=" + strconv.Itoa(d.PageSize)
}

func (d logsDataTest) LiveURL() string {
	v := url.Values{}
	v.Set("view", "log-table")
	v.Set("file", d.CurrentFile)
	if d.Level != "" {
		v.Set("level", d.Level)
	}
	if d.Search != "" {
		v.Set("search", d.Search)
	}
	v.Set("page", strconv.Itoa(d.Page))
	v.Set("pageSize", strconv.Itoa(d.PageSize))
	return "/logs?" + v.Encode()
}

func (d logsDataTest) PrevPage() int {
	if d.Page > 1 {
		return d.Page - 1
	}
	return 1
}

func (d logsDataTest) NextPage() int {
	if d.Page < d.TotalPages {
		return d.Page + 1
	}
	return d.TotalPages
}

type logsEntryTest struct {
	Time    string
	Level   string
	Message string
	Context string
}

func TestRenderLogEntriesPagination(t *testing.T) {
	tmpl := NewTemplate()

	data := logsDataTest{
		Entries: []logsEntryTest{
			{Time: "2026-08-23 10:00:00", Level: "INFO", Message: "older"},
			{Time: "2026-08-23 11:00:00", Level: "ERROR", Message: "newer", Context: `{"key":"value"}`},
		},
		CurrentFile:  "gorgon-2026-08-23.log",
		Page:         2,
		PageSize:     100,
		TotalPages:   3,
		TotalEntries: 250,
	}

	var buf bytes.Buffer
	if err := tmpl.templates.ExecuteTemplate(&buf, "log-entries", PageData{Data: data}); err != nil {
		t.Fatalf("failed to render log-entries: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		"logs-pagination",
		"Page 2 of 3",
		"250 entries",
		"page=1&amp;pageSize=100",
		"page=3&amp;pageSize=100",
	} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("rendered log-entries missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderLogEntriesPollingOnlyOnFirstPageOfCurrentFile(t *testing.T) {
	tmpl := NewTemplate()

	render := func(data logsDataTest) string {
		var buf bytes.Buffer
		if err := tmpl.templates.ExecuteTemplate(&buf, "log-entries", PageData{Data: data}); err != nil {
			t.Fatalf("failed to render log-entries: %v", err)
		}
		return buf.String()
	}

	firstPageCurrent := render(logsDataTest{
		Entries:       []logsEntryTest{{Level: "INFO", Message: "x"}},
		CurrentFile:   "gorgon-2026-08-23.log",
		Page:          1,
		PageSize:      100,
		TotalPages:    5,
		IsCurrentFile: true,
	})
	if !strings.Contains(firstPageCurrent, `hx-trigger="every 5s"`) {
		t.Error("page 1 of current file should include polling trigger")
	}

	// The poller must live on the table region only; the container that wraps
	// the pagination controls must stay static so clicks are never destroyed
	// by a polling swap.
	if !strings.Contains(firstPageCurrent, `id="logs-live"`) {
		t.Error("polling element #logs-live is missing")
	}
	for _, line := range strings.Split(firstPageCurrent, "\n") {
		if strings.Contains(line, `id="logs-entries-container"`) && strings.Contains(line, "hx-") {
			t.Errorf("container must not carry hx attributes (poll would destroy pagination clicks): %s", strings.TrimSpace(line))
		}
	}
	if !strings.Contains(firstPageCurrent, `hx-get="/logs?file=gorgon-2026-08-23.log&amp;page=1&amp;pageSize=100&amp;view=log-table"`) &&
		!strings.Contains(firstPageCurrent, `view=log-table`) {
		t.Error("poller should request the log-table partial, not the full log-entries partial")
	}

	otherPage := render(logsDataTest{
		Entries:       []logsEntryTest{{Level: "INFO", Message: "x"}},
		CurrentFile:   "gorgon-2026-08-23.log",
		Page:          3,
		PageSize:      100,
		TotalPages:    5,
		IsCurrentFile: true,
	})
	if strings.Contains(otherPage, "hx-trigger") {
		t.Error("pages other than 1 should not include polling trigger")
	}

	oldFile := render(logsDataTest{
		Entries:       []logsEntryTest{{Level: "INFO", Message: "x"}},
		CurrentFile:   "gorgon-2026-08-01.log",
		Page:          1,
		PageSize:      100,
		TotalPages:    2,
		IsCurrentFile: false,
	})
	if strings.Contains(oldFile, "hx-trigger") {
		t.Error("non-current files should not include polling trigger")
	}
}
