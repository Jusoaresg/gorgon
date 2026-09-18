package views

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"strings"
	"time"

	"github.com/jusoaresg/gorgon/internal/episode/model"
	episodeModel "github.com/jusoaresg/gorgon/internal/episode/model"
	"github.com/labstack/echo/v4"
)

type Template struct {
	templates *template.Template
}

type PageData struct {
	TemplateName string
	Data         any
	Styles       []string
}

func (t *Template) Render(w io.Writer, name string, data any, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func NewTemplate() *Template {
	tmpl := template.New("")

	tmpl.Funcs(template.FuncMap{
		"toLower": func(text string) string {
			return strings.ToLower(text)
		},
		"extractYear": func(dateStr string) string {
			if len(dateStr) >= 4 {
				return dateStr[:4]
			}
			return dateStr
		},
		"render": func(name string, data any) (template.HTML, error) {
			var buf bytes.Buffer
			err := tmpl.ExecuteTemplate(&buf, name, data)
			return template.HTML(buf.String()), err
		},
		"safeHtml": func(s string) template.HTML {
			return template.HTML(s)
		},
		"derefFloat": func(f *float64) float64 {
			if f == nil {
				return 0
			}
			return *f
		},
		"derefInt64": func(i *int64) int64 {
			if i == nil {
				return 0
			}
			return *i
		},
		"countDownloaded": func(episodes []episodeModel.Episode) int {
			count := 0
			for _, ep := range episodes {
				if ep.Tracking == episodeModel.TrackingDownloaded {
					count++
				}
			}
			return count
		},
		"countMonitored": func(episodes []episodeModel.Episode) int {
			count := 0
			for _, ep := range episodes {
				if ep.Tracking != episodeModel.TrackingSkipped {
					count++
				}
			}
			return count
		},
		"calcPercentage": func(part, total int) int {
			if total == 0 {
				return 0
			}
			return int((float64(part) / float64(total)) * 100)
		},
		"seasonDownloadedCount": func(seasonNumber int, episodes []episodeModel.Episode) int {
			count := 0
			for _, ep := range episodes {
				if ep.Season == seasonNumber && ep.Tracking == episodeModel.TrackingDownloaded {
					count++
				}
			}
			return count
		},
		"seasonTotalCount": func(seasonNumber int, episodes []episodeModel.Episode) int {
			count := 0
			for _, ep := range episodes {
				if ep.Season == seasonNumber {
					count++
				}
			}
			return count
		},
		"dict": func(values ...any) (map[string]any, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("invalid dict call")
			}
			dict := make(map[string]any, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings")
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
		"airDate": func(e model.Episode) string { return e.AirDate() },
		"airTimeUnix": func(airstamp int64) string {
			return time.Unix(airstamp, 0).UTC().Format("15:04")
		},
		"nowDate": func() string {
			return time.Now().UTC().Format("2006-01-02")
		},
		"formatBytes": func(value any) string {
			var size float64
			switch v := value.(type) {
			case int:
				size = float64(v)
			case int64:
				size = float64(v)
			case float64:
				size = v
			default:
				return "0 B"
			}
			return formatSizeBytes(size)
		},
		"formatSpeed": func(bytesPerSec any) string {
			var bps float64
			switch v := bytesPerSec.(type) {
			case int:
				bps = float64(v)
			case int64:
				bps = float64(v)
			case float64:
				bps = v
			default:
				return "0 B/s"
			}
			return formatSizeBytes(bps) + "/s"
		},
		"formatEta": func(seconds int) string {
			if seconds <= 0 || seconds >= 8640000 {
				return "∞"
			}
			h := seconds / 3600
			m := (seconds % 3600) / 60
			s := seconds % 60
			if h > 0 {
				return fmt.Sprintf("%dh %dm", h, m)
			}
			if m > 0 {
				return fmt.Sprintf("%dm %ds", m, s)
			}
			return fmt.Sprintf("%ds", s)
		},
		"toPercent": func(progress float32) int {
			pct := int(progress * 100)
			if pct < 0 {
				return 0
			}
			if pct > 100 {
				return 100
			}
			return pct
		},
		"torrentStateLabel": func(state string) string {
			switch state {
			case "downloading", "forcedDL":
				return "Downloading"
			case "metaDL":
				return "Fetching Metadata"
			case "queuedDL":
				return "Queued"
			case "stalledDL":
				return "Stalled"
			case "pausedDL":
				return "Paused"
			case "checkingDL", "checkingResumeData":
				return "Checking"
			case "allocating":
				return "Allocating"
			case "moving":
				return "Moving"
			case "uploading", "stalledUP", "queuedUP", "pausedUP", "forcedUP", "checkingUP":
				return "Waiting to Import"
			default:
				return "Unknown"
			}
		},
		"torrentStateClass": func(state string) string {
			switch state {
			case "downloading", "forcedDL":
				return "downloading"
			case "metaDL":
				return "metadata"
			case "queuedDL", "pausedDL":
				return "queued"
			case "stalledDL":
				return "stalled"
			case "checkingDL", "checkingResumeData", "allocating":
				return "checking"
			case "moving":
				return "moving"
			case "uploading", "stalledUP", "queuedUP", "pausedUP", "forcedUP", "checkingUP":
				return "waiting"
			default:
				return "paused"
			}
		},
	})

	tmpl, err := tmpl.ParseFS(
		FrontFS,
		"ui/*.html",
		"ui/components/*.html",
		"ui/components/settings/*.html",
		"ui/components/show/*.html",
	)
	if err != nil {
		panic(fmt.Errorf("error while loading embedded front %w", err))
	}

	return &Template{
		templates: tmpl,
	}
}

func formatSizeBytes(size float64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%.0f B", size)
	}
	div, exp := float64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", size/div, "KMGTPE"[exp])
}
