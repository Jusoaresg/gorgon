-- +goose Up
-- +goose StatementBegin

ALTER TABLE episode_torrents ADD COLUMN is_manual INTEGER NOT NULL DEFAULT 0;
ALTER TABLE episode_torrents ADD COLUMN previous_title TEXT NOT NULL DEFAULT '';
ALTER TABLE episode_torrents ADD COLUMN previous_indexer TEXT NOT NULL DEFAULT '';
ALTER TABLE episode_torrents ADD COLUMN previous_info_url TEXT NOT NULL DEFAULT '';
ALTER TABLE episode_torrents ADD COLUMN previous_publish_date TEXT NOT NULL DEFAULT '';
ALTER TABLE episode_torrents ADD COLUMN previous_score INTEGER NOT NULL DEFAULT 0;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE episode_torrents DROP COLUMN previous_score;
ALTER TABLE episode_torrents DROP COLUMN previous_publish_date;
ALTER TABLE episode_torrents DROP COLUMN previous_info_url;
ALTER TABLE episode_torrents DROP COLUMN previous_indexer;
ALTER TABLE episode_torrents DROP COLUMN previous_title;
ALTER TABLE episode_torrents DROP COLUMN is_manual;

-- +goose StatementEnd
