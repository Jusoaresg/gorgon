package web

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMondayOfWeek(t *testing.T) {
	cases := []struct {
		name     string
		now      time.Time
		expected time.Time
	}{
		{
			name:     "middle of the week",
			now:      time.Date(2026, 7, 29, 15, 30, 0, 0, time.UTC),
			expected: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "monday itself",
			now:      time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC),
			expected: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "sunday",
			now:      time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC),
			expected: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "month boundary",
			now:      time.Date(2026, 8, 1, 2, 0, 0, 0, time.UTC),
			expected: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "year boundary",
			now:      time.Date(2026, 1, 4, 2, 0, 0, 0, time.UTC),
			expected: time.Date(2025, 12, 29, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mondayOfWeek(tc.now, tc.now.Location())
			if !got.Equal(tc.expected) {
				t.Errorf("mondayOfWeek(%s) = %s, want %s", tc.now, got, tc.expected)
			}
		})
	}
}

func TestMondayOfWeekBuildsMondayToSunday(t *testing.T) {
	monday := mondayOfWeek(time.Date(2026, 8, 1, 2, 0, 0, 0, time.UTC), time.UTC)
	if got, want := monday.Weekday(), time.Monday; got != want {
		t.Fatalf("week start = %s, want %s", got, want)
	}
	for i := range 7 {
		day := monday.AddDate(0, 0, i)
		want := (time.Monday + time.Weekday(i)) % 7
		if day.Weekday() != want {
			t.Errorf("day %d = %s, want %s", i, day.Weekday(), want)
		}
	}
}

func TestComputeWeekStartWithParam(t *testing.T) {
	got := computeWeekStart("2026-08-01", time.UTC)
	want := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("computeWeekStart(\"2026-08-01\", time.UTC) = %s, want %s", got, want)
	}
}

func TestComputeWeekStartCurrentWeek(t *testing.T) {
	got := computeWeekStart("", time.UTC)
	want := mondayOfWeek(time.Now().UTC(), time.UTC)
	if !got.Equal(want) {
		t.Errorf("computeWeekStart(\"\", time.UTC) = %s, want %s", got, want)
	}
}

func TestComputeWeekStartTimezone(t *testing.T) {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Skip("America/Sao_Paulo not available")
	}
	// Sunday night in Brazil: 2026-09-20 22:00 BRT
	// In UTC, this is 2026-09-21 01:00 (Monday UTC).
	// In Brazil, Sunday is part of the week starting on Monday 2026-09-14!
	sundayNightBRT := time.Date(2026, 9, 20, 22, 0, 0, 0, loc)
	mondayBRT := mondayOfWeek(sundayNightBRT, loc)
	expectedMonday := time.Date(2026, 9, 14, 0, 0, 0, 0, loc)
	if !mondayBRT.Equal(expectedMonday) {
		t.Errorf("mondayOfWeek on Sunday night in BRT = %s, want %s", mondayBRT, expectedMonday)
	}
}

func makeLogEntries(n int) []LogEntry {
	entries := make([]LogEntry, n)
	for i := range entries {
		entries[i] = LogEntry{Message: "entry", Level: "INFO"}
	}
	return entries
}

func TestPaginateLogs(t *testing.T) {
	cases := []struct {
		name           string
		total          int
		page           int
		pageSize       int
		wantLen        int
		wantPage       int
		wantTotalPages int
	}{
		{name: "first page", total: 250, page: 1, pageSize: 100, wantLen: 100, wantPage: 1, wantTotalPages: 3},
		{name: "middle page", total: 250, page: 2, pageSize: 100, wantLen: 100, wantPage: 2, wantTotalPages: 3},
		{name: "partial last page", total: 250, page: 3, pageSize: 100, wantLen: 50, wantPage: 3, wantTotalPages: 3},
		{name: "page beyond end clamps to last", total: 250, page: 99, pageSize: 100, wantLen: 50, wantPage: 3, wantTotalPages: 3},
		{name: "page below one clamps to first", total: 250, page: -5, pageSize: 100, wantLen: 100, wantPage: 1, wantTotalPages: 3},
		{name: "single partial page", total: 10, page: 1, pageSize: 100, wantLen: 10, wantPage: 1, wantTotalPages: 1},
		{name: "empty entries", total: 0, page: 1, pageSize: 100, wantLen: 0, wantPage: 1, wantTotalPages: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, gotPage, gotTotalPages := paginateLogs(makeLogEntries(tc.total), tc.page, tc.pageSize)
			if len(got) != tc.wantLen {
				t.Errorf("len(entries) = %d, want %d", len(got), tc.wantLen)
			}
			if gotPage != tc.wantPage {
				t.Errorf("page = %d, want %d", gotPage, tc.wantPage)
			}
			if gotTotalPages != tc.wantTotalPages {
				t.Errorf("totalPages = %d, want %d", gotTotalPages, tc.wantTotalPages)
			}
		})
	}
}

func writeLogFile(t *testing.T, path string, lines []string) {
	t.Helper()
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write log file: %v", err)
	}
}

func TestLoadLogEntriesCacheInvalidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gorgon-2026-08-23.log")

	writeLogFile(t, path, []string{
		`{"time":"2026-08-23T10:00:00Z","level":"INFO","msg":"older"}`,
		`{"time":"2026-08-23T11:00:00Z","level":"ERROR","msg":"newer"}`,
	})

	first := loadLogEntries(path)
	if len(first) != 2 {
		t.Fatalf("got %d entries, want 2", len(first))
	}
	if first[0].Message != "newer" {
		t.Errorf("entries not newest first: first message = %q", first[0].Message)
	}

	cached := loadLogEntries(path)
	if &cached[0] != &first[0] {
		t.Error("second load should return the cached slice")
	}

	writeLogFile(t, path, []string{
		`{"time":"2026-08-23T12:00:00Z","level":"WARN","msg":"newest"}`,
	})
	refreshed := loadLogEntries(path)
	if len(refreshed) != 1 || refreshed[0].Message != "newest" {
		t.Errorf("cache was not invalidated after file change: %+v", refreshed)
	}
}

func TestLoadLogEntriesGzip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gorgon-2026-08-22.log")

	f, err := os.Create(path + ".gz")
	if err != nil {
		t.Fatalf("failed to create gzip file: %v", err)
	}
	gz := gzip.NewWriter(f)
	if _, err := gz.Write([]byte(`{"time":"2026-08-22T09:00:00Z","level":"DEBUG","msg":"gzipped"}` + "\n")); err != nil {
		t.Fatalf("failed to write gzip content: %v", err)
	}
	gz.Close()
	f.Close()

	entries := loadLogEntries(path)
	if len(entries) != 1 || entries[0].Message != "gzipped" {
		t.Errorf("unexpected entries from gz file: %+v", entries)
	}
}
