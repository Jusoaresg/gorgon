-- +goose Up
CREATE TABLE release_deletion_operations (
 id TEXT PRIMARY KEY,
 target TEXT NOT NULL,
 episode_id INTEGER NOT NULL DEFAULT 0,
 hash TEXT NOT NULL DEFAULT '',
 intention TEXT NOT NULL,
 status TEXT NOT NULL,
 stage TEXT NOT NULL,
 manifest TEXT NOT NULL,
 attempts INTEGER NOT NULL DEFAULT 0,
 last_error TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL,
 updated_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX release_deletion_active_target ON release_deletion_operations(target) WHERE status != 'completed';
-- +goose Down
DROP TABLE release_deletion_operations;
