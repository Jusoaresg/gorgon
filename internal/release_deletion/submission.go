package release_deletion

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// Coordinator permits contract tests and alternate entry points to delegate to
// the same application boundary. Production dispatch uses managed submission.
type Coordinator interface {
	Delete(context.Context, Request) (Operation, error)
}

func Dispatch(ctx context.Context, svc Coordinator, r Request) (Operation, error) {
	if submitter, ok := svc.(interface {
		Submit(context.Context, Request) (Operation, error)
	}); ok {
		return submitter.Submit(ctx, r)
	}
	return svc.Delete(ctx, r)
}

// Submit records the request without network calls or waiting for execution.
// The managed worker verifies and persists the manifest before any deletion.
func (s *Service) Submit(ctx context.Context, r Request) (Operation, error) {
	var op Operation
	if err := ctx.Err(); err != nil {
		return op, err
	}
	switch r.Intention {
	case "skipped", "wanted", "missing", "blacklist":
	default:
		return op, ErrUnsafe
	}
	r.Hash = strings.ToLower(strings.TrimSpace(r.Hash))
	if r.EpisodeID != 0 {
		var hash string
		err := s.DB.Get(&hash, "SELECT hash FROM episode_torrents WHERE episode_id=?", r.EpisodeID)
		if errors.Is(err, sql.ErrNoRows) {
			err = s.DB.Get(&op, "SELECT * FROM release_deletion_operations WHERE episode_id=? ORDER BY created_at DESC,id DESC LIMIT 1", r.EpisodeID)
			if err == nil {
				if op.Intention != r.Intention {
					return op, ErrConflict
				}
				return op, nil
			}
		}
		if err != nil {
			return op, ErrUnsafe
		}
		if r.Hash != "" && !strings.EqualFold(hash, r.Hash) {
			return op, ErrConflict
		}
		r.Hash = strings.ToLower(hash)
	}
	if r.Hash == "" {
		return op, ErrUnsafe
	}
	var ids []int64
	if err := s.DB.Select(&ids, "SELECT episode_id FROM episode_torrents WHERE hash=? COLLATE NOCASE", r.Hash); err != nil {
		return op, err
	}
	if len(ids) > 1 {
		return op, ErrUnsafe
	}
	if len(ids) == 1 {
		if r.EpisodeID != 0 && r.EpisodeID != ids[0] {
			return op, ErrUnsafe
		}
		r.EpisodeID = ids[0]
	}
	target := "hash:" + r.Hash
	err := s.DB.Get(&op, "SELECT * FROM release_deletion_operations WHERE target=? ORDER BY created_at DESC,id DESC LIMIT 1", target)
	if err == nil {
		if op.Status != "completed" {
			if op.Intention != r.Intention || op.EpisodeID != r.EpisodeID {
				return op, ErrConflict
			}
			return op, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return op, err
	}
	requestJSON, err := json.Marshal(r)
	if err != nil {
		return op, err
	}
	now := s.now().Unix()
	op = Operation{ID: uuid.NewString(), Target: target, EpisodeID: r.EpisodeID, Hash: r.Hash, Intention: r.Intention, Status: "pending", Stage: "queued", ManifestJSON: string(requestJSON), CreatedAt: s.now().UnixMicro(), UpdatedAt: now, Version: 1, Retryable: true}
	_, err = s.DB.NamedExec(`INSERT INTO release_deletion_operations(id,target,episode_id,hash,intention,status,stage,manifest,attempts,last_error,created_at,updated_at,version) VALUES(:id,:target,:episode_id,:hash,:intention,:status,:stage,:manifest,:attempts,:last_error,:created_at,:updated_at,:version)`, op)
	if err != nil {
		var existing Operation
		if e := s.DB.Get(&existing, "SELECT * FROM release_deletion_operations WHERE target=? AND status!='completed'", target); e == nil {
			if existing.Intention != r.Intention || existing.EpisodeID != r.EpisodeID {
				return existing, ErrConflict
			}
			return existing, nil
		}
		return op, err
	}
	s.notify(op)
	return op, nil
}

func (s *Service) SubmitRetry(ctx context.Context, id string) (Operation, error) {
	if err := ctx.Err(); err != nil {
		return Operation{}, err
	}
	// This atomic transition never rewinds an executing operation or completed result.
	_, err := s.DB.ExecContext(ctx, `UPDATE release_deletion_operations SET status='pending',retry_count=0,next_attempt_at=0,last_error='',updated_at=?,version=version+1 WHERE id=? AND status='failed'`, s.now().Unix(), id)
	if err != nil {
		return Operation{}, err
	}
	op, err := s.Get(id)
	if err == nil {
		s.notify(op)
	}
	return op, err
}
func HTTPStatus(op Operation, err error) int {
	if errors.Is(err, ErrConflict) {
		return http.StatusConflict
	}
	if errors.Is(err, ErrUnsafe) {
		return http.StatusUnprocessableEntity
	}
	if errors.Is(err, sql.ErrNoRows) {
		return http.StatusNotFound
	}
	if err != nil {
		return http.StatusInternalServerError
	}
	if op.Status == "completed" {
		return http.StatusOK
	}
	if op.Status == "failed" {
		return http.StatusConflict
	}
	return http.StatusAccepted
}
func Response(op Operation, err error) map[string]any {
	message := "Release deletion accepted; cleanup is pending"
	code := "deletion_pending"
	switch {
	case errors.Is(err, ErrConflict):
		message = "Another deletion operation has a conflicting intention"
		code = "deletion_conflict"
	case errors.Is(err, ErrUnsafe):
		message = "Deletion was blocked because ownership, shared files, or filesystem paths could not be verified"
		code = "unsafe_deletion_target"
	case errors.Is(err, sql.ErrNoRows):
		message = "Deletion operation was not found"
		code = "operation_not_found"
	case err != nil || op.Status == "failed":
		message = "Release deletion is incomplete; inspect its status and retry when resolved"
		code = "deletion_incomplete"
	case op.Status == "completed":
		message = "Release deleted"
		code = "deletion_completed"
	}
	return map[string]any{"operation": op, "toastMessage": message, "code": code}
}
