-- +goose Up
CREATE TABLE release_replacement_searches (
 operation_id TEXT PRIMARY KEY REFERENCES release_deletion_operations(id) ON DELETE CASCADE,
 episode_id INTEGER NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
 status TEXT NOT NULL DEFAULT 'pending',
 attempts INTEGER NOT NULL DEFAULT 0,
 next_attempt_at INTEGER NOT NULL DEFAULT 0,
 last_error TEXT NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE release_replacement_searches;
