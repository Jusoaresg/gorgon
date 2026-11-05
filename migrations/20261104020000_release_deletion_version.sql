-- +goose Up
ALTER TABLE release_deletion_operations ADD COLUMN version INTEGER NOT NULL DEFAULT 0;
-- +goose Down
ALTER TABLE release_deletion_operations DROP COLUMN version;
