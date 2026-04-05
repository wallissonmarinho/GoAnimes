-- +goose Up
CREATE TABLE IF NOT EXISTS catalog_series (
  id TEXT NOT NULL PRIMARY KEY,
  name TEXT NOT NULL,
  poster TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  genres_json TEXT NOT NULL DEFAULT '[]',
  release_info TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS catalog_item (
  id TEXT NOT NULL PRIMARY KEY,
  series_id TEXT NOT NULL REFERENCES catalog_series(id) ON DELETE CASCADE,
  type TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL DEFAULT '',
  poster TEXT NOT NULL DEFAULT '',
  magnet_url TEXT NOT NULL DEFAULT '',
  torrent_url TEXT NOT NULL DEFAULT '',
  info_hash TEXT NOT NULL DEFAULT '',
  released TEXT NOT NULL DEFAULT '',
  subtitles_tag TEXT NOT NULL DEFAULT '',
  series_name TEXT NOT NULL DEFAULT '',
  season INTEGER NOT NULL DEFAULT 0,
  episode INTEGER NOT NULL DEFAULT 0,
  is_special BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_catalog_item_series_id ON catalog_item(series_id);
CREATE INDEX IF NOT EXISTS idx_catalog_item_released ON catalog_item(released);

-- +goose Down
DROP TABLE IF EXISTS catalog_item;
DROP TABLE IF EXISTS catalog_series;
