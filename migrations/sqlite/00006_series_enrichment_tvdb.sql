-- +goose Up
ALTER TABLE series_enrichment ADD COLUMN tvdb_series_id INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE series_enrichment DROP COLUMN tvdb_series_id;
