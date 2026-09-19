package web

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jusoaresg/gorgon/config"
	filterProfileModel "github.com/jusoaresg/gorgon/internal/filter_profile/model"
	filterProfileRepository "github.com/jusoaresg/gorgon/internal/filter_profile/repository"
	filterSettingsModel "github.com/jusoaresg/gorgon/internal/filter_settings/model"
	filterSettingsRepository "github.com/jusoaresg/gorgon/internal/filter_settings/repository"
	"github.com/jusoaresg/gorgon/internal/show/service"
	"github.com/jusoaresg/gorgon/views"
	"github.com/labstack/echo/v4"
)

type ShowsGrid struct {
	Shows   []service.AggregatedShow
	Filters struct {
		Search string
		Status string
		Sort   string
	}
}

type ShowsGridData struct {
	Data ShowsGrid
}

func (h *Handler) ShowsRoute(c echo.Context) error {
	search := c.QueryParam("search")
	status := c.QueryParam("status")
	sort := c.QueryParam("sort")
	if sort == "" {
		sort = "added"
	}

	shows, err := h.AggregatorService.ListFullShowsFiltered(search, status)
	if err != nil {
		return err
	}
	showsGrid := ShowsGrid{
		Shows: shows,
		Filters: struct {
			Search string
			Status string
			Sort   string
		}{
			Search: search,
			Status: status,
			Sort:   sort,
		},
	}

	SortShows(shows, sort)

	return views.Render(c, views.View{
		Layout:    "layout",
		Default:   "shows",
		Templates: map[string]string{"grid": "shows-grid-htmx"},
		Data:      showsGrid,
		Styles:    []string{"shows.css"},
	})
}

func (h *Handler) ShowRoute(c echo.Context) error {
	showId, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return err
	}

	show, err := h.AggregatorService.GetShowWithRelationsById(showId)
	if err != nil {
		return err
	}

	return views.Render(c, views.View{
		Layout:  "layout",
		Default: "show",
		Data:    show,
		Styles:  []string{"show.css"},
	})
}

func (h *Handler) AddShowRoute(c echo.Context) error {
	return views.Render(c, views.View{
		Layout:  "layout",
		Default: "add-show",
		Data:    nil,
		Styles:  []string{"add-show.css"},
	})
}

func (h *Handler) AddShowConfigRoute(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("tvmaze-id"), 10, 64)
	if err != nil {
		return err
	}

	show, err := h.TvMazeService.SearchByTvMazeId(id)
	if err != nil {
		return err
	}

	return views.Render(c, views.View{
		Layout:  "layout",
		Default: "add-show-config",
		Data:    show,
		Styles:  []string{"add-show-config.css"},
	})
}

func computeWeekStart(weekParam string) time.Time {
	now := time.Now().UTC()
	if weekParam != "" {
		parsed, err := time.Parse("2006-01-02", weekParam)
		if err == nil {
			return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC)
		}
	}
	return mondayOfWeek(now)
}

func mondayOfWeek(now time.Time) time.Time {
	daysSinceMonday := int(now.Weekday()+6) % 7
	monday := now.AddDate(0, 0, -daysSinceMonday)
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
}

func (h *Handler) computeCalendarData(weekParam string) (CalendarData, error) {
	weekStart := computeWeekStart(weekParam)
	weekEnd := weekStart.AddDate(0, 0, 7)
	today := time.Now().UTC().Format("2006-01-02")

	var episodes []CalendarEpisode
	err := h.DB.Select(&episodes, `
		SELECT e.id, e.show_id, e.name, e.number, e.season, e.airstamp, e.tracking,
		       s.name AS show_name, s.image_medium AS show_image, s.status AS show_status
		FROM episodes e
		JOIN shows s ON e.show_id = s.id
		WHERE e.airstamp >= ? AND e.airstamp < ?
		ORDER BY e.airstamp ASC
	`, weekStart.Unix(), weekEnd.Unix())
	if err != nil {
		return CalendarData{}, err
	}

	days := make([]CalendarDay, 7)
	for i := range 7 {
		day := weekStart.AddDate(0, 0, i)
		days[i] = CalendarDay{
			Date:        day.Format("2006-01-02"),
			DayName:     day.Format("Monday"),
			DisplayDate: day.Format("2 Jan"),
			Episodes:    []CalendarEpisode{},
		}
	}

	for _, ep := range episodes {
		t := time.Unix(ep.AirStamp, 0).UTC()
		dayIdx := int(t.Sub(weekStart) / (24 * time.Hour))
		if dayIdx >= 0 && dayIdx < 7 {
			days[dayIdx].Episodes = append(days[dayIdx].Episodes, ep)
		}
	}

	currentWeekStart := computeWeekStart("")
	isCurrentWeek := weekStart.Equal(currentWeekStart)

	return CalendarData{
		Days:          days,
		WeekStart:     weekStart.Format("Jan 2"),
		WeekEnd:       weekEnd.AddDate(0, 0, -1).Format("Jan 2, 2006"),
		PrevWeek:      weekStart.AddDate(0, 0, -7).Format("2006-01-02"),
		NextWeek:      weekStart.AddDate(0, 0, 7).Format("2006-01-02"),
		Today:         today,
		IsCurrentWeek: isCurrentWeek,
	}, nil
}

func (h *Handler) CalendarRoute(c echo.Context) error {
	data, err := h.computeCalendarData(c.QueryParam("week"))
	if err != nil {
		return err
	}

	return views.Render(c, views.View{
		Layout:    "layout",
		Default:   "calendar",
		Templates: map[string]string{"week": "calendar-week"},
		Data:      data,
		Styles:    []string{"calendar.css"},
	})
}

func (h *Handler) CalendarWeekHTMX(c echo.Context) error {
	data, err := h.computeCalendarData(c.QueryParam("week"))
	if err != nil {
		return err
	}

	return c.Render(200, "calendar-week", views.PageData{Data: data})
}

type SettingType int

const (
	GorgonSettings SettingType = iota
	ProwlarrSettings
	TorrentSettings
	TelegramSettings
	FilterSettings
)

var SettingTypes = map[string]SettingType{
	"gorgon":   GorgonSettings,
	"prowlarr": ProwlarrSettings,
	"torrent":  TorrentSettings,
	"telegram": TelegramSettings,
	"filter":   FilterSettings,
}

type SettingsData struct {
	Type       SettingType
	TypeString string
	Settings   any
}

type FilterSettingsData struct {
	FilterSettings filterSettingsModel.FilterSettings
	Profiles       []filterProfileModel.FilterProfile
}

func (h *Handler) SettingsRoute(c echo.Context) error {
	return c.Redirect(http.StatusSeeOther, "/settings/gorgon")
}

func (h *Handler) SettingsTypeRoute(c echo.Context) error {
	settingsType := c.Param("type")

	settingType, ok := SettingTypes[settingsType]
	if !ok {
		settingType = GorgonSettings
		settingsType = "gorgon"
	}

	var settings any
	if settingType == FilterSettings {
		settingsRepo := filterSettingsRepository.NewFilterSettingsRepository(h.DB)
		filterSettings, err := settingsRepo.Get()
		if err != nil {
			return err
		}

		profileRepo := filterProfileRepository.NewFilterProfileRepository(h.DB)
		profiles, err := profileRepo.List()
		if err != nil {
			return err
		}

		settings = FilterSettingsData{
			FilterSettings: filterSettings,
			Profiles:       profiles,
		}
	} else {
		cfg, err := config.LoadConfig()
		if err != nil {
			return views.Render(c, views.View{
				Layout:  "layout",
				Default: "settings",
				Data:    nil,
				Styles:  []string{"settings.css"},
			})
		}
		settings = cfg
	}

	return views.Render(c, views.View{
		Layout:  "layout",
		Default: "settings",
		Data: SettingsData{
			Type:       settingType,
			TypeString: strings.Join([]string{settingsType, "Settings"}, ""),
			Settings:   settings,
		},
		Styles: []string{"settings.css"},
	})
}

type LogEntry struct {
	Time    string
	Level   string
	Message string
	Context string
}

type LogFileInfo struct {
	Name      string
	Size      string
	IsCurrent bool
}

type LogsData struct {
	Files         []LogFileInfo
	Entries       []LogEntry
	CurrentFile   string
	Search        string
	Level         string
	Page          int
	PageSize      int
	TotalPages    int
	TotalEntries  int
	IsCurrentFile bool
}

func (d LogsData) BaseQuery() string {
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

// PageURL returns the full /logs URL for the given page of log entries,
// preserving the current file/search/level filters.
func (d LogsData) PageURL(page int) string {
	return "/logs?" + d.BaseQuery() + "&page=" + strconv.Itoa(page) + "&pageSize=" + strconv.Itoa(d.PageSize)
}

// LiveURL returns the partial-refresh URL used by the auto-refresh poller.
// It renders only the log table (not the pagination controls) so that polling
// never replaces elements the user is interacting with.
func (d LogsData) LiveURL() string {
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

func (d LogsData) PrevPage() int {
	if d.Page > 1 {
		return d.Page - 1
	}
	return 1
}

func (d LogsData) NextPage() int {
	if d.Page < d.TotalPages {
		return d.Page + 1
	}
	return d.TotalPages
}

var logFileNamePattern = regexp.MustCompile(`^gorgon-[A-Za-z0-9._-]+\.log(?:\.gz)?$`)

func isValidLogFileName(name string) bool {
	if name == "" {
		return false
	}
	if filepath.IsAbs(name) {
		return false
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return false
	}
	return logFileNamePattern.MatchString(name)
}

func (h *Handler) LogsRoute(c echo.Context) error {
	logsPath := config.LogsPath

	search := c.QueryParam("search")
	levelFilter := c.QueryParam("level")
	selectedFile := c.QueryParam("file")

	defaultFile := fmt.Sprintf("gorgon-%s.log", time.Now().In(time.FixedZone("BRT", -3*60*60)).Format("2006-01-02"))
	if selectedFile == "" || !isValidLogFileName(selectedFile) {
		selectedFile = defaultFile
	}
	isCurrentFile := selectedFile == defaultFile

	if !isValidLogFileName(selectedFile) {
		return c.String(400, "invalid log file")
	}

	page := 1
	if p, err := strconv.Atoi(c.QueryParam("page")); err == nil && p > 0 {
		page = p
	}
	const defaultPageSize = 100
	const maxPageSize = 500
	pageSize := defaultPageSize
	if ps, err := strconv.Atoi(c.QueryParam("pageSize")); err == nil && ps > 0 {
		pageSize = ps
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	entries := loadLogEntries(filepath.Join(logsPath, selectedFile))
	entries = filterLogs(entries, search, levelFilter)

	totalEntries := len(entries)
	pageEntries, page, totalPages := paginateLogs(entries, page, pageSize)

	var files []LogFileInfo
	entriesDir, err := os.ReadDir(logsPath)
	if err == nil {
		for _, f := range entriesDir {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			if !strings.HasPrefix(name, "gorgon-") {
				continue
			}
			info, _ := f.Info()
			size := info.Size()
			sizeStr := formatSize(size)
			isCurrent := name == fmt.Sprintf("gorgon-%s.log", time.Now().In(time.FixedZone("BRT", -3*60*60)).Format("2006-01-02"))
			if strings.HasSuffix(name, ".gz") {
				isCurrent = false
			}
			files = append(files, LogFileInfo{
				Name:      name,
				Size:      sizeStr,
				IsCurrent: isCurrent,
			})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Name > files[j].Name
	})

	return views.Render(c, views.View{
		Layout:    "layout",
		Default:   "logs",
		Templates: map[string]string{"log-entries": "log-entries", "log-table": "log-table"},
		Data: LogsData{
			Files:         files,
			Entries:       pageEntries,
			CurrentFile:   selectedFile,
			Search:        search,
			Level:         levelFilter,
			Page:          page,
			PageSize:      pageSize,
			TotalPages:    totalPages,
			TotalEntries:  totalEntries,
			IsCurrentFile: isCurrentFile,
		},
		Styles: []string{"logs.css"},
	})
}

// paginateLogs slices entries (newest first) for the requested page, clamping
// page to the valid range. Returns the page slice, the effective page and the
// total number of pages.
func paginateLogs(entries []LogEntry, page, pageSize int) ([]LogEntry, int, int) {
	totalPages := (len(entries) + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}

	start := (page - 1) * pageSize
	end := start + pageSize
	if end > len(entries) {
		end = len(entries)
	}

	var pageEntries []LogEntry
	if start < end {
		pageEntries = entries[start:end]
	}
	return pageEntries, page, totalPages
}

type logFileCacheEntry struct {
	entries []LogEntry
	modTime time.Time
	size    int64
}

var (
	logCacheMutex sync.Mutex
	logCache      = map[string]logFileCacheEntry{}
)

// maxLogCacheFiles bounds the number of cached files, evicting the oldest one
// so the cache cannot grow indefinitely as log files rotate.
const maxLogCacheFiles = 32

func evictOldestLogCacheLocked() {
	if len(logCache) < maxLogCacheFiles {
		return
	}
	var oldestPath string
	var oldestTime time.Time
	for path, entry := range logCache {
		if oldestPath == "" || entry.modTime.Before(oldestTime) {
			oldestPath = path
			oldestTime = entry.modTime
		}
	}
	delete(logCache, oldestPath)
}

// loadLogEntries returns the parsed entries of a log file (plain or gzipped),
// newest first. Parsed entries are cached and only re-parsed when the file's
// mtime or size changes.
func loadLogEntries(path string) []LogEntry {
	info, err := os.Stat(path)
	if err != nil {
		path += ".gz"
		info, err = os.Stat(path)
		if err != nil {
			return nil
		}
	}

	logCacheMutex.Lock()
	defer logCacheMutex.Unlock()
	if cached, ok := logCache[path]; ok && cached.modTime.Equal(info.ModTime()) && cached.size == info.Size() {
		return cached.entries
	}

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []LogEntry
	if strings.HasSuffix(path, ".gz") {
		gzr, err := gzip.NewReader(f)
		if err != nil {
			return nil
		}
		defer gzr.Close()
		entries = parseLogLines(gzr)
	} else {
		entries = parseLogLines(f)
	}

	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	evictOldestLogCacheLocked()
	logCache[path] = logFileCacheEntry{entries: entries, modTime: info.ModTime(), size: info.Size()}
	return entries
}

func parseLogLines(r io.Reader) []LogEntry {
	var entries []LogEntry
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Bytes()
		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		entry := LogEntry{}

		if t, ok := raw["time"].(string); ok {
			parsed, err := time.Parse(time.RFC3339Nano, t)
			if err == nil {
				entry.Time = parsed.Format("2006-01-02 15:04:05")
			} else {
				entry.Time = t
			}
		}
		if l, ok := raw["level"].(string); ok {
			entry.Level = l
		}
		if m, ok := raw["msg"].(string); ok {
			entry.Message = m
		}

		delete(raw, "time")
		delete(raw, "level")
		delete(raw, "msg")

		if len(raw) > 0 {
			b, _ := json.Marshal(raw)
			entry.Context = string(b)
		}

		entries = append(entries, entry)
	}
	return entries
}

func filterLogs(entries []LogEntry, search, level string) []LogEntry {
	if search == "" && level == "" {
		return entries
	}
	var filtered []LogEntry
	for _, e := range entries {
		if level != "" && !strings.EqualFold(e.Level, level) {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(e.Message), strings.ToLower(search)) && !strings.Contains(e.Context, search) {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

func formatSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
}

// INFO: Helpers
func SortShows(shows []service.AggregatedShow, sortBy string) {
	switch sortBy {
	case "name":
		sort.Slice(shows, func(i, j int) bool {
			return shows[i].Show.Name < shows[j].Show.Name
		})
	case "next":
		sort.Slice(shows, func(i, j int) bool {
			return nextEpisodeTime(shows[i]) < nextEpisodeTime(shows[j])
		})
	case "added":
		sort.Slice(shows, func(i, j int) bool {
			return shows[i].Show.Updated < shows[j].Show.Updated
		})
	}
}

func nextEpisodeTime(show service.AggregatedShow) int64 {
	var next int64 = 1<<63 - 1 // maior int64 possível
	now := time.Now().Unix()

	for _, ep := range show.Episodes {
		if ep.AirStamp > now && ep.AirStamp < next {
			next = ep.AirStamp
		}
	}
	return next
}
