package filter

import (
	"path/filepath"
	"regexp"
	"strings"
)

type ReleaseMetadata struct {
	ReleaseGroup string
	Resolution   string
	Source       string
	Codec        string
	Audio        string
	Languages    []string
}

var (
	// Anime prefix group: [Erai-raws], [SubsPlease], [Judas], etc.
	animeGroupPrefixRe = regexp.MustCompile(`^\[([a-zA-Z0-9_\-\s\.\+]+)\]`)

	// Standard scene suffix group: -DIMENSION, -FLUX, -GGEZ, -YIFY, etc.
	sceneGroupSuffixRe = regexp.MustCompile(`[-–—]([a-zA-Z0-9]+)(?:\.[a-zA-Z0-9]{2,4})?$`)

	// Resolution patterns
	resolutionRegex = regexp.MustCompile(`(?i)\b(2160p|1080p|720p|480p|360p|4k|8k|uhd)\b`)

	// Source patterns
	sourceRegex = regexp.MustCompile(`(?i)\b(web-?dl|web-?rip|web|bluray|blu-ray|bdrip|brrip|hdtv|dvdrip|dvd)\b`)

	// Codec patterns
	codecRegex = regexp.MustCompile(`(?i)\b(x265|h265|hevc|x264|h264|avc|av1|xvid|10bit|8bit)\b`)

	// Audio patterns
	audioRegex = regexp.MustCompile(`(?i)\b(5\.1|7\.1|2\.0|aac|ac3|eac3|dts-hd|dts|flac|opus|mp3)\b`)

	// Language / subtitle tags
	multiSubRegex  = regexp.MustCompile(`(?i)\b(multiple[\s\._\-]*subtitles?|multi[\s\._\-]*subs?|multisub)\b`)
	ptbrRegex      = regexp.MustCompile(`(?i)\b(pt[\s\._\-]*br|pob|portuguese|legendado|dublado)\b`)
	dualAudioRegex = regexp.MustCompile(`(?i)\b(dual[\s\._\-]*audio|multi[\s\._\-]*audio)\b`)
)

// ParseReleaseMetadata extracts structured metadata from a release filename.
func ParseReleaseMetadata(rawName string) ReleaseMetadata {
	name := strings.TrimSpace(rawName)
	ext := filepath.Ext(name)
	cleanName := strings.TrimSuffix(name, ext)

	meta := ReleaseMetadata{}

	// 1. Extract Release Group
	if m := animeGroupPrefixRe.FindStringSubmatch(cleanName); len(m) > 1 {
		groupCandidate := strings.TrimSpace(m[1])
		// Avoid treating generic words in brackets like [1080p] or [Batch] as group
		if !resolutionRegex.MatchString(groupCandidate) && !codecRegex.MatchString(groupCandidate) {
			meta.ReleaseGroup = groupCandidate
		}
	} else if m := sceneGroupSuffixRe.FindStringSubmatch(cleanName); len(m) > 1 {
		groupCandidate := strings.TrimSpace(m[1])
		if !resolutionRegex.MatchString(groupCandidate) && !codecRegex.MatchString(groupCandidate) {
			meta.ReleaseGroup = groupCandidate
		}
	}

	// 2. Extract Resolution
	if m := resolutionRegex.FindString(cleanName); m != "" {
		meta.Resolution = strings.ToUpper(m)
	}

	// 3. Extract Source
	if m := sourceRegex.FindString(cleanName); m != "" {
		meta.Source = strings.ToUpper(m)
	}

	// 4. Extract Codec
	if m := codecRegex.FindString(cleanName); m != "" {
		meta.Codec = strings.ToUpper(m)
	}

	// 5. Extract Audio
	if m := audioRegex.FindString(cleanName); m != "" {
		meta.Audio = strings.ToUpper(m)
	}

	// 6. Extract Languages / Subs
	if multiSubRegex.MatchString(cleanName) {
		meta.Languages = append(meta.Languages, "Multi-Sub")
	}
	if ptbrRegex.MatchString(cleanName) {
		meta.Languages = append(meta.Languages, "PT-BR")
	}
	if dualAudioRegex.MatchString(cleanName) {
		meta.Languages = append(meta.Languages, "Dual Audio")
	}

	return meta
}
