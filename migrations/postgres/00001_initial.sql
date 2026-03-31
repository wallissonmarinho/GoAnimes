-- +goose Up
CREATE TABLE IF NOT EXISTS rss_sources (
  id TEXT NOT NULL PRIMARY KEY,
  url TEXT NOT NULL UNIQUE,
  label TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS catalog_snapshot (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  items_json JSONB NOT NULL DEFAULT '[]',
  ok BOOLEAN NOT NULL DEFAULT FALSE,
  message TEXT NOT NULL DEFAULT '',
  item_count INTEGER NOT NULL DEFAULT 0,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ
);

INSERT INTO catalog_snapshot (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS catalog_snapshot;
DROP TABLE IF EXISTS rss_sources;
