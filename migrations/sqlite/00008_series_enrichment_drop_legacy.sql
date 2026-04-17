-- +goose Up
DROP INDEX IF EXISTS idx_series_enrichment_kitsu_anime_id;

ALTER TABLE series_enrichment DROP COLUMN anilist_media_id;
ALTER TABLE series_enrichment DROP COLUMN kitsu_anime_id;
ALTER TABLE series_enrichment DROP COLUMN anidb_aid;
ALTER TABLE series_enrichment DROP COLUMN anidb_last_fetch_unix;

-- +goose Down
ALTER TABLE series_enrichment ADD COLUMN anilist_media_id INTEGER;
ALTER TABLE series_enrichment ADD COLUMN kitsu_anime_id TEXT NOT NULL DEFAULT '';
ALTER TABLE series_enrichment ADD COLUMN anidb_aid INTEGER NOT NULL DEFAULT 0;
ALTER TABLE series_enrichment ADD COLUMN anidb_last_fetch_unix INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_series_enrichment_kitsu_anime_id ON series_enrichment(kitsu_anime_id) WHERE kitsu_anime_id <> '';
