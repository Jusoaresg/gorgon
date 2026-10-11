package service

import (
	"strings"

	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
)

func getQualityScore(t schema.SearchResponse) int {
	possibleQuality := make(map[string]int)
	possibleQuality["2560"] = 20
	possibleQuality["1080"] = 20
	possibleQuality["720"] = 10
	possibleQuality["480"] = 0

	name := t.Filename
	if name == "" {
		name = t.Title
	}

	for key, quality := range possibleQuality {
		if strings.Contains(name, key) {
			return quality
		}
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
