package release_deletion

import (
	"context"
	"errors"
	"github.com/jusoaresg/gorgon/internal/release_guard"
	"os"
	"time"

	"github.com/jusoaresg/gorgon/internal/episode/events"
)

const MaxAutomaticAttempts = 5

var retryDelays = []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute}

func (s *Service) now() time.Time {
	if s.Options.Now != nil {
		return s.Options.Now()
	}
	return time.Now()
}
func retryable(err error) bool {
	return !errors.Is(err, ErrUnsafe) && !errors.Is(err, ErrConflict) && !errors.Is(err, os.ErrPermission)
}
func (s *Service) List() ([]Operation, error) {
	ops := []Operation{}
	err := s.DB.Select(&ops, "SELECT * FROM release_deletion_operations WHERE intention != 'upgrade' ORDER BY created_at DESC, id DESC")
	return ops, err
}

// Retry gives an actionable or exhausted operation another bounded retry budget.
// The same manifest and intention are retained; completed operations remain unchanged.
func (s *Service) Retry(ctx context.Context, id string) (Operation, error) {
	unlock := release_guard.Lock(s.DB)
	defer unlock()
	op, err := s.Get(id)
	if err != nil {
		return op, err
	}
	if op.Status == "completed" {
		return op, nil
	}
	op.RetryCount = 0
	op.NextAttemptAt = 0
	return s.run(ctx, op)
}

// RecoverDue reconciles persisted operations after restart and on each polling tick.
// An operation failure is persisted by run and does not prevent other targets recovering.
func (s *Service) RecoverDue(ctx context.Context, now time.Time) error {
	var ids []string
	err := s.DB.Select(&ids, `SELECT id FROM release_deletion_operations WHERE status IN ('pending','running') OR (status='failed' AND retryable=1 AND next_attempt_at>0 AND next_attempt_at<=?) ORDER BY created_at,id`, now.Unix())
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := s.Resume(ctx, id); err != nil && s.Logger != nil {
			s.Logger.Warn("release deletion recovery remains incomplete", "operation_id", id)
		}
	}
	return nil
}

// Run performs startup recovery then keeps retrying due work until cancellation.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.RecoverDue(ctx, s.now()); err != nil && s.Logger != nil {
			s.Logger.Error("release deletion recovery failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Service) notify(op Operation) {
	if op.Intention == "upgrade" {
		return
	}
	if op.Status == "completed" && op.EpisodeID != 0 {
		tracking := op.Intention
		if tracking == "blacklist" {
			tracking = "wanted"
		}
		episode.EmitEpisodeTrackingUpdatedEvent(op.EpisodeID, tracking, "")
	}
	if s.Options.OnChange != nil {
		s.Options.OnChange(op)
	}
}
