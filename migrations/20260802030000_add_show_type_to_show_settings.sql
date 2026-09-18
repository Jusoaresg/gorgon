-- +goose Up
-- +goose StatementBegin

ALTER TABLE show_settings ADD COLUMN show_type TEXT NOT NULL DEFAULT 'standard';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE show_settings DROP COLUMN show_type;

-- +goose StatementEnd
