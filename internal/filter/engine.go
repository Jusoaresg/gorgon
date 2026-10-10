package filter

type PreferredPattern struct {
	Pattern string
	Score   int
}

type Profile struct {
	ID                int64
	Name              string
	UpgradeUntilScore int
	Search            []string
	Required          []string
	Rejected          []string
	Preferred         []PreferredPattern
}

type Result struct {
	Passed         bool
	RejectedReason string
	PreferredScore int
}

// Validate compiles every pattern in the profile against ctx, returning the
// first compilation error (if any). Useful to reject bad patterns at save time.
func Validate(profile *Profile, ctx Context) error {
	if profile == nil {
		return nil
	}

	for _, pattern := range profile.Required {
		if _, err := Compile(pattern, ctx); err != nil {
			return err
		}
	}
	for _, pattern := range profile.Rejected {
		if _, err := Compile(pattern, ctx); err != nil {
			return err
		}
	}
	for _, pattern := range profile.Preferred {
		if _, err := Compile(pattern.Pattern, ctx); err != nil {
			return err
		}
	}

	return nil
}

// Evaluate applies the profile gates to a release filename:
//   - all required patterns must match (a compilation failure rejects);
//   - no rejected pattern may match (rejected wins over required);
//   - preferred patterns never reject, each match adds its score.
func Evaluate(profile *Profile, ctx Context, filename string) Result {
	if profile == nil {
		return Result{Passed: true}
	}

	for _, pattern := range profile.Required {
		re, err := Compile(pattern, ctx)
		if err != nil {
			return Result{Passed: false, RejectedReason: "required (compile error): " + pattern}
		}
		if !re.MatchString(filename) {
			return Result{Passed: false, RejectedReason: "required: " + pattern}
		}
	}

	for _, pattern := range profile.Rejected {
		re, err := Compile(pattern, ctx)
		if err != nil {
			continue
		}
		if re.MatchString(filename) {
			return Result{Passed: false, RejectedReason: "rejected: " + pattern}
		}
	}

	score := 0
	for _, preferred := range profile.Preferred {
		re, err := Compile(preferred.Pattern, ctx)
		if err != nil {
			continue
		}
		if re.MatchString(filename) {
			score += preferred.Score
		}
	}

	return Result{Passed: true, PreferredScore: score}
}
