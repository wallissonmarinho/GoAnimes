-- +goose Up
ALTER TABLE series_enrichment ADD COLUMN IF NOT EXISTS tvdb_series_id INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE series_enrichment DROP COLUMN IF EXISTS tvdb_series_id;
