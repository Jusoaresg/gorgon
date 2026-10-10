package api

import (
	"errors"
	"fmt"
	"strings"

	filterProfileModel "github.com/jusoaresg/gorgon/internal/filter_profile/model"
	"github.com/jusoaresg/gorgon/internal/filter_profile/schema"
)

func validateSaveProfileRequest(request schema.SaveFilterProfileRequest) error {
	if strings.TrimSpace(request.Name) == "" {
		return errors.New("name is required")
	}

	if request.UpgradeUntilScore < 0 {
		return errors.New("upgrade_until_score cannot be negative")
	}

	for i, pattern := range request.Patterns {
		if strings.TrimSpace(pattern.Pattern) == "" {
			return fmt.Errorf("pattern %d: pattern is required", i)
		}

		switch filterProfileModel.FilterPatternKind(pattern.Kind) {
		case filterProfileModel.KindSearch,
			filterProfileModel.KindRequired,
			filterProfileModel.KindRejected,
			filterProfileModel.KindPreferred:
		default:
			return fmt.Errorf("pattern %d: invalid kind %q", i, pattern.Kind)
		}

		if pattern.Score < 0 {
			return fmt.Errorf("pattern %d: score cannot be negative", i)
		}
	}

	return nil
}
