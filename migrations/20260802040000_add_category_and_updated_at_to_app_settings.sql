-- +goose Up
-- +goose StatementBegin

ALTER TABLE app_settings ADD COLUMN category TEXT NOT NULL DEFAULT '';
ALTER TABLE app_settings ADD COLUMN updated_at INTEGER NOT NULL DEFAULT 0;

UPDATE app_settings SET updated_at = unixepoch() WHERE updated_at = 0;
UPDATE app_settings SET category = 'filters' WHERE key = 'filters';

CREATE INDEX IF NOT EXISTS idx_app_settings_category ON app_settings(category);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_app_settings_category;

-- +goose StatementEnd
