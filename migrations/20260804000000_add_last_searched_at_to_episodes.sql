-- +goose Up
-- +goose StatementBegin
ALTER TABLE episodes ADD COLUMN last_searched_at INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE episodes DROP COLUMN last_searched_at;
-- +goose StatementEnd
