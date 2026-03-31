-- +goose Up
CREATE TABLE IF NOT EXISTS rss_sources (
  id TEXT NOT NULL PRIMARY KEY,
  url TEXT NOT NULL UNIQUE,
  label TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS catalog_snapshot (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  items_json TEXT NOT NULL DEFAULT '[]',
  ok INTEGER NOT NULL DEFAULT 0,
  message TEXT NOT NULL DEFAULT '',
  item_count INTEGER NOT NULL DEFAULT 0,
  started_at TEXT,
  finished_at TEXT
);

INSERT OR IGNORE INTO catalog_snapshot (id) VALUES (1);

-- +goose Down
DROP TABLE IF EXISTS catalog_snapshot;
DROP TABLE IF EXISTS rss_sources;
