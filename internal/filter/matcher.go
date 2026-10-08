package filter

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	// Standard TV patterns: S01E02, S1E2, 1x02, S01.E02, S01E02-E04, S01E02-04
	standardSeasonEpRe = regexp.MustCompile(`(?i)(?:[sS](\d{1,2})[\s._\-]?[eE](\d{1,3})(?:-[eE]?(\d{1,3}))?|(\d{1,2})x(\d{1,3}))`)

	// Universal season patterns: S2, Season 2, Season.2, 2nd Season
	seasonRe = regexp.MustCompile(`(?i)(?:season[.\s_\-]*(\d{1,2})|\b[sS](\d{1,2})\b|(\d{1,2})[.\s_\-]*(?:st|nd|rd|th)?[.\s_\-]*season)`)

	// Standard standalone episode patterns (no season prefix): E02, E02-E04, E02-04, Episode 02, Ep. 02, Part 02
	standardEpOnlyRe = regexp.MustCompile(`(?i)\b(?:[eE]|ep|episode|part)[.\s_-]*(\d{1,4})(?:-(?:[eE]|ep|episode|part)?[.\s_-]*(\d{1,4}))?\b`)

	// Anime episode patterns: " - 12", " - 05v2", " Ep 12", " Episode 12", "[12]"
	// We require a separator like hyphen or "ep"/"episode" before the number to avoid matching years/resolutions.
	animeEpisodeHyphenRe  = regexp.MustCompile(`(?i)(?:[\s_]|^)[-–—]\s*(\d{1,4})(?:v\d+)?(?:\s|\[|\.|\(|$)`)
	animeEpisodeWordRe    = regexp.MustCompile(`(?i)(?:[\s_\[\(]|^)(?:ep|episode)\.?\s*(\d{1,4})(?:v\d+)?(?:\s|\]|\)|\.|\-|$)`)
	animeEpisodeBracketRe = regexp.MustCompile(`\[(\d{1,4})(?:v\d+)?\]`)

	// Strips out bracketed CRC32 hashes like [26EF07AE] or [12345678]
	crcHashRe = regexp.MustCompile(`\[[0-9a-fA-F]{8}\]`)

	// Common non-episode numeric tokens
	resolutionRe = regexp.MustCompile(`(?i)\b(?:2160|1080|720|576|480|360)[pi]\b`)
	yearRe       = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	codecRe      = regexp.MustCompile(`(?i)\b(?:x264|x265|h264|h265|hevc|avc|av1|xvid|10bit|8bit)\b`)
	audioRe      = regexp.MustCompile(`(?i)\b(?:5\.1|7\.1|2\.0|aac|ac3|eac3|dts|flac|opus|mp3)\b`)
)

// MatchEpisode checks whether releaseName belongs to the given season and episode.
// If showType is "anime", it supports both standard SxxExx notations and anime notations (e.g. " - 12").
// If no season is specified in the release, it is assumed to be Season 1 only.
func MatchEpisode(releaseName string, season, episode int, showType string) bool {
	cleanName := sanitizeReleaseName(releaseName)
	if cleanName == "" {
		return false
	}

	// 1. Try standard SxxExx / 1x02 match
	if match, s, epStart, epEnd := extractStandardSeasonEp(cleanName); match {
		if s == season {
			if epEnd > 0 {
				return episode >= epStart && episode <= epEnd
			}
			return episode == epStart
		}
		// If explicit season doesn't match, this is definitely a wrong release
		return false
	}

	// Clean out resolutions, codecs, audio tokens, years so numbers are not confused
	cleaned := cleanTokens(cleanName)

	// 2. Check for explicit season in release name (e.g. "Season 2", "S2", "2nd Season")
	hasExplicitSeason, detectedSeason := extractExplicitSeason(cleaned)
	if hasExplicitSeason && detectedSeason != season {
		return false
	}

	// 3. If no season explicitly specified in the release, assume Season 1 only
	if !hasExplicitSeason && season != 1 {
		return false
	}

	// 4. Match episode
	if showType == "anime" {
		return matchAnimeEpisode(cleaned, episode)
	}

	return matchStandardEpisode(cleaned, episode)
}

func sanitizeReleaseName(name string) string {
	base := filepath.Base(name)
	ext := filepath.Ext(base)
	if len(ext) > 0 {
		base = strings.TrimSuffix(base, ext)
	}
	// Remove CRC hashes to avoid matching numeric hashes
	base = crcHashRe.ReplaceAllString(base, "")
	return base
}

func cleanTokens(name string) string {
	cleaned := resolutionRe.ReplaceAllString(name, " ")
	cleaned = codecRe.ReplaceAllString(cleaned, " ")
	cleaned = audioRe.ReplaceAllString(cleaned, " ")
	cleaned = yearRe.ReplaceAllString(cleaned, " ")
	return cleaned
}

func extractStandardSeasonEp(name string) (bool, int, int, int) {
	matches := standardSeasonEpRe.FindAllStringSubmatch(name, -1)
	if len(matches) == 0 {
		return false, 0, 0, 0
	}

	// Use the last match if multiple (usually filename at the end is most specific)
	match := matches[len(matches)-1]
	if match[1] != "" && match[2] != "" {
		s, _ := strconv.Atoi(match[1])
		epStart, _ := strconv.Atoi(match[2])
		epEnd := 0
		if match[3] != "" {
			epEnd, _ = strconv.Atoi(match[3])
		}
		return true, s, epStart, epEnd
	} else if match[4] != "" && match[5] != "" {
		s, _ := strconv.Atoi(match[4])
		ep, _ := strconv.Atoi(match[5])
		return true, s, ep, 0
	}

	return false, 0, 0, 0
}

func extractExplicitSeason(name string) (bool, int) {
	seasonMatches := seasonRe.FindAllStringSubmatch(name, -1)
	for _, sm := range seasonMatches {
		for i := 1; i <= 3; i++ {
			if sm[i] != "" {
				s, err := strconv.Atoi(sm[i])
				if err == nil && s > 0 {
					return true, s
				}
			}
		}
	}
	return false, 0
}

func extractStandardEpOnly(name string) (bool, int, int) {
	matches := standardEpOnlyRe.FindAllStringSubmatch(name, -1)
	if len(matches) == 0 {
		return false, 0, 0
	}

	// Use the last match if multiple
	match := matches[len(matches)-1]
	if match[1] != "" {
		epStart, _ := strconv.Atoi(match[1])
		epEnd := 0
		if match[2] != "" {
			epEnd, _ = strconv.Atoi(match[2])
		}
		return true, epStart, epEnd
	}

	return false, 0, 0
}

func matchStandardEpisode(name string, targetEpisode int) bool {
	if match, epStart, epEnd := extractStandardEpOnly(name); match {
		if epEnd > 0 {
			return targetEpisode >= epStart && targetEpisode <= epEnd
		}
		return targetEpisode == epStart
	}
	return false
}

func matchAnimeEpisode(name string, targetEpisode int) bool {
	// 1. Try standard episode notation (e.g. E05, Ep. 05, Episode 05)
	if match, epStart, epEnd := extractStandardEpOnly(name); match {
		if epEnd > 0 {
			return targetEpisode >= epStart && targetEpisode <= epEnd
		}
		return targetEpisode == epStart
	}

	// 2. Try " - 12" style (most common in Erai-raws, SubsPlease, HorribleSubs, etc.)
	if m := animeEpisodeHyphenRe.FindStringSubmatch(name); len(m) > 1 {
		ep, err := strconv.Atoi(m[1])
		if err == nil {
			return ep == targetEpisode
		}
	}

	// 3. Try "Episode 12" or "Ep. 12" style
	if m := animeEpisodeWordRe.FindStringSubmatch(name); len(m) > 1 {
		ep, err := strconv.Atoi(m[1])
		if err == nil {
			return ep == targetEpisode
		}
	}

	// 4. Try "[12]" style
	if m := animeEpisodeBracketRe.FindStringSubmatch(name); len(m) > 1 {
		ep, err := strconv.Atoi(m[1])
		if err == nil {
			return ep == targetEpisode
		}
	}

	return false
}
