-- +goose Up
ALTER TABLE release_deletion_operations ADD COLUMN next_attempt_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE release_deletion_operations ADD COLUMN retry_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE release_deletion_operations ADD COLUMN retryable INTEGER NOT NULL DEFAULT 1;
CREATE INDEX release_deletion_recovery ON release_deletion_operations(status,next_attempt_at);
-- +goose Down
DROP INDEX release_deletion_recovery;
ALTER TABLE release_deletion_operations DROP COLUMN retryable;
ALTER TABLE release_deletion_operations DROP COLUMN retry_count;
ALTER TABLE release_deletion_operations DROP COLUMN next_attempt_at;
