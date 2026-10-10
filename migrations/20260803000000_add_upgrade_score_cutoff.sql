-- +goose Up
-- +goose StatementBegin
ALTER TABLE episode_torrents ADD COLUMN score INTEGER NOT NULL DEFAULT 0;
ALTER TABLE filter_profiles ADD COLUMN upgrade_until_score INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE episode_torrents DROP COLUMN score;
ALTER TABLE filter_profiles DROP COLUMN upgrade_until_score;
-- +goose StatementEnd
