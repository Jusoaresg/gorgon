package service

import (
	"strings"

	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
)

// releaseName returns the release filename, falling back to title if filename is empty.
func releaseName(t schema.SearchResponse) string {
	if t.Filename != "" {
		return t.Filename
	}
	return t.Title
}

func getQualityScore(t schema.SearchResponse) int {
	name := strings.ToLower(releaseName(t))

	if strings.Contains(name, "2160") || strings.Contains(name, "4k") || strings.Contains(name, "uhd") {
		return 30
	}
	if strings.Contains(name, "1080") || strings.Contains(name, "2560") {
		return 20
	}
	if strings.Contains(name, "720") {
		return 10
	}
	if strings.Contains(name, "480") {
		return 0
	}
	return 0
}

func getHealthScore(t schema.SearchResponse) int {
	health := t.Seeders - t.Leechers
	if t.Seeders == 0 {
		health = -50
	} else if health > 30 {
		health = 30
	} else if health < 0 {
		health = 0
	}
	return health
}

// BaseQualityScore calculates intrinsic release quality (resolution only).
func BaseQualityScore(t schema.SearchResponse) int {
	return getQualityScore(t)
}

// BaseScore provides backward compatibility combining quality and swarm health.
func BaseScore(t schema.SearchResponse) int {
	return getQualityScore(t) + getHealthScore(t)
}

func baseScore(t schema.SearchResponse) int {
	return BaseScore(t)
}
