-- +goose Up
-- +goose StatementBegin

CREATE TABLE release_blacklist (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	show_id INTEGER NOT NULL,
	episode_id INTEGER,
	info_hash TEXT NOT NULL DEFAULT '',
	release_title TEXT NOT NULL DEFAULT '',
	indexer TEXT NOT NULL DEFAULT '',
	reason TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL DEFAULT (unixepoch()),
	expires_at INTEGER,

	FOREIGN KEY(show_id) REFERENCES shows(id) ON DELETE CASCADE,
	FOREIGN KEY(episode_id) REFERENCES episodes(id) ON DELETE CASCADE
);

CREATE INDEX idx_release_blacklist_show_id ON release_blacklist(show_id);
CREATE INDEX idx_release_blacklist_episode_id ON release_blacklist(episode_id);
CREATE INDEX idx_release_blacklist_hash ON release_blacklist(info_hash COLLATE NOCASE);

ALTER TABLE episode_torrents ADD COLUMN stalled_since INTEGER NOT NULL DEFAULT 0;
ALTER TABLE episode_torrents ADD COLUMN last_amount_left INTEGER NOT NULL DEFAULT -1;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE episode_torrents DROP COLUMN last_amount_left;
ALTER TABLE episode_torrents DROP COLUMN stalled_since;

DROP INDEX IF EXISTS idx_release_blacklist_hash;
DROP INDEX IF EXISTS idx_release_blacklist_episode_id;
DROP INDEX IF EXISTS idx_release_blacklist_show_id;
DROP TABLE IF EXISTS release_blacklist;

-- +goose StatementEnd
