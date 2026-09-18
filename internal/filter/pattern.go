package filter

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	DefaultSearchPattern            = "{alias} S{season:00}E{episode:00}"
	DefaultAnimeSearchPattern       = "{alias} - {episode:00}"
	DefaultAnimeSeasonSearchPattern = "{alias} S{season} - {episode:00}"
)

// Context carries the information used to expand placeholders in a pattern.
// Names should already be normalized before being passed in.
type Context struct {
	Show     string
	Aliases  []string
	Season   int
	Episode  int
	ShowType string
}


// AllNames returns the canonical show name followed by every alias.
func (c Context) AllNames() []string {
	return c.names()
}

var placeholderRe = regexp.MustCompile(`\{([a-zA-Z]+)(?::(\d+))?\}`)

// Compile expands placeholders in pattern against ctx and returns the
// compiled case-insensitive regexp used to match release filenames.
//
// Supported placeholders:
//   - {alias}     any known name (canonical show name or any alias)
//   - {show}      only the canonical show name
//   - {season}    season number, no padding
//   - {season:00} season number zero-padded to the given width
//   - {episode}   episode number, no padding
//   - {episode:00} episode number zero-padded to the given width
//
// Letters and separators around placeholders are literal (e.g. the "S" and
// "E" in "S{season:00}E{episode:00}").
func Compile(pattern string, ctx Context) (*regexp.Regexp, error) {
	var sb strings.Builder
	sb.WriteString("(?i)")

	index := 0
	for _, match := range placeholderRe.FindAllStringSubmatchIndex(pattern, -1) {
		start, end := match[0], match[1]
		if start > index {
			sb.WriteString(regexp.QuoteMeta(pattern[index:start]))
		}

		name := pattern[match[2]:match[3]]

		width := 0
		if match[4] != -1 {
			width = len(pattern[match[4]:match[5]])
		}

		expansion, err := expandPlaceholder(name, width, ctx)
		if err != nil {
			return nil, err
		}
		sb.WriteString(expansion)

		index = end
	}

	if index < len(pattern) {
		sb.WriteString(regexp.QuoteMeta(pattern[index:]))
	}

	re, err := regexp.Compile(sb.String())
	if err != nil {
		return nil, fmt.Errorf("failed to compile pattern %q: %w", pattern, err)
	}
	return re, nil
}

func expandPlaceholder(name string, width int, ctx Context) (string, error) {
	switch name {
	case "alias":
		names := ctx.names()
		if len(names) == 0 {
			return "", fmt.Errorf("{alias} used but no names available")
		}
		parts := make([]string, 0, len(names))
		for _, n := range names {
			parts = append(parts, regexp.QuoteMeta(n))
		}
		return "(?:" + strings.Join(parts, "|") + ")", nil
	case "show":
		return regexp.QuoteMeta(ctx.Show), nil
	case "season":
		return formatNumber(ctx.Season, width), nil
	case "episode", "ep":
		return formatNumber(ctx.Episode, width), nil
	case "absolute":
		return "", fmt.Errorf("placeholder {absolute} is reserved but not implemented yet")
	default:
		return "", fmt.Errorf("unknown placeholder {%s}", name)
	}
}

func formatNumber(n int, width int) string {
	s := strconv.Itoa(n)
	if width > len(s) {
		return strings.Repeat("0", width-len(s)) + s
	}
	return s
}

func (c Context) names() []string {
	names := make([]string, 0, len(c.Aliases)+1)
	if c.Show != "" {
		names = append(names, c.Show)
	}
	for _, alias := range c.Aliases {
		if alias == "" || alias == c.Show {
			continue
		}
		names = append(names, alias)
	}
	return names
}
