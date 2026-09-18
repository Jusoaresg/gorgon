package filter

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	// Standard TV patterns: S01E02, S1E2, 1x02, S01E02-E04, S01E02-04
	standardSeasonEpRe = regexp.MustCompile(`(?i)(?:[sS](\d{1,2})[eE](\d{1,3})(?:-[eE]?(\d{1,3}))?|(\d{1,2})x(\d{1,3}))`)

	// Anime season patterns: S2, Season 2, 2nd Season
	animeSeasonRe = regexp.MustCompile(`(?i)(?:season\s*(\d{1,2})|\b[sS](\d{1,2})\b|(\d{1,2})(?:st|nd|rd|th)\s*season)`)

	// Anime episode patterns: " - 12", " - 05v2", " Ep 12", " Episode 12", "[12]"
	// We require a separator like hyphen or "ep"/"episode" before the number to avoid matching years/resolutions.
	animeEpisodeHyphenRe = regexp.MustCompile(`(?i)(?:[\s_]|^)[-–—]\s*(\d{1,4})(?:v\d+)?(?:\s|\[|\.|\(|$)`)
	animeEpisodeWordRe   = regexp.MustCompile(`(?i)(?:[\s_\[\(]|^)(?:ep|episode)\.?\s*(\d{1,4})(?:v\d+)?(?:\s|\]|\)|\.|\-|$)`)
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

	// If standard mode and no SxxExx match, reject
	if showType != "anime" {
		return false
	}

	// 2. Anime-specific matching
	return matchAnimeRelease(cleanName, season, episode)
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

func matchAnimeRelease(name string, targetSeason, targetEpisode int) bool {
	// Check if title specifies a season
	detectedSeason := 1
	hasExplicitSeason := false

	seasonMatches := animeSeasonRe.FindAllStringSubmatch(name, -1)
	for _, sm := range seasonMatches {
		for i := 1; i <= 3; i++ {
			if sm[i] != "" {
				s, err := strconv.Atoi(sm[i])
				if err == nil && s > 0 {
					detectedSeason = s
					hasExplicitSeason = true
					break
				}
			}
		}
	}

	// If explicit season was detected, it must match targetSeason
	if hasExplicitSeason && detectedSeason != targetSeason {
		return false
	}

	// If no season explicitly in title, we only assume season 1
	if !hasExplicitSeason && targetSeason > 1 {
		// Season > 1 with no season indicator could be absolute episode numbering
		// We still try to match the episode number directly
	}

	// Clean out resolutions, codecs, audio tokens from the string so numbers like "1080" or "264" are not confused
	cleaned := resolutionRe.ReplaceAllString(name, " ")
	cleaned = codecRe.ReplaceAllString(cleaned, " ")
	cleaned = audioRe.ReplaceAllString(cleaned, " ")
	cleaned = yearRe.ReplaceAllString(cleaned, " ")

	// 1. Try " - 12" style (most common in Erai-raws, SubsPlease, HorribleSubs, etc.)
	if m := animeEpisodeHyphenRe.FindStringSubmatch(cleaned); len(m) > 1 {
		ep, err := strconv.Atoi(m[1])
		if err == nil {
			return ep == targetEpisode
		}
	}

	// 2. Try "Episode 12" or "Ep. 12" style
	if m := animeEpisodeWordRe.FindStringSubmatch(cleaned); len(m) > 1 {
		ep, err := strconv.Atoi(m[1])
		if err == nil {
			return ep == targetEpisode
		}
	}

	// 3. Try "[12]" style
	if m := animeEpisodeBracketRe.FindStringSubmatch(cleaned); len(m) > 1 {
		ep, err := strconv.Atoi(m[1])
		if err == nil {
			return ep == targetEpisode
		}
	}

	return false
}
