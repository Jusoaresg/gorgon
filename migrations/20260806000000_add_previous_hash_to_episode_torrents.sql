-- +goose Up
-- +goose StatementBegin

ALTER TABLE episode_torrents ADD COLUMN previous_hash TEXT NOT NULL DEFAULT '';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE episode_torrents DROP COLUMN previous_hash;

-- +goose StatementEnd
