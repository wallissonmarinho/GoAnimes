-- +goose Up
DROP INDEX IF EXISTS idx_series_enrichment_kitsu_anime_id;

ALTER TABLE series_enrichment DROP COLUMN IF EXISTS anilist_media_id;
ALTER TABLE series_enrichment DROP COLUMN IF EXISTS kitsu_anime_id;
ALTER TABLE series_enrichment DROP COLUMN IF EXISTS anidb_aid;
ALTER TABLE series_enrichment DROP COLUMN IF EXISTS anidb_last_fetch_unix;

-- +goose Down
ALTER TABLE series_enrichment ADD COLUMN IF NOT EXISTS anilist_media_id INTEGER;
ALTER TABLE series_enrichment ADD COLUMN IF NOT EXISTS kitsu_anime_id TEXT NOT NULL DEFAULT '';
ALTER TABLE series_enrichment ADD COLUMN IF NOT EXISTS anidb_aid INTEGER NOT NULL DEFAULT 0;
ALTER TABLE series_enrichment ADD COLUMN IF NOT EXISTS anidb_last_fetch_unix BIGINT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_series_enrichment_kitsu_anime_id ON series_enrichment(kitsu_anime_id) WHERE kitsu_anime_id <> '';
