-- +goose Up
CREATE TABLE IF NOT EXISTS series_enrichment (
  series_id TEXT NOT NULL PRIMARY KEY REFERENCES catalog_series(id) ON DELETE CASCADE,
  anilist_media_id INTEGER,
  mal_id INTEGER NOT NULL DEFAULT 0,
  imdb_id TEXT NOT NULL DEFAULT '',
  kitsu_anime_id TEXT NOT NULL DEFAULT '',
  anidb_aid INTEGER NOT NULL DEFAULT 0,
  anidb_last_fetch_unix INTEGER NOT NULL DEFAULT 0,
  al_search_ver INTEGER NOT NULL DEFAULT 0,
  next_air_unix INTEGER NOT NULL DEFAULT 0,
  next_air_ep INTEGER NOT NULL DEFAULT 0,
  next_air_from_al INTEGER NOT NULL DEFAULT 0,
  start_year INTEGER NOT NULL DEFAULT 0,
  episode_length_min INTEGER NOT NULL DEFAULT 0,
  poster_url TEXT NOT NULL DEFAULT '',
  background_url TEXT NOT NULL DEFAULT '',
  al_banner_url TEXT NOT NULL DEFAULT '',
  hero_bg_url TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  trailer_youtube_id TEXT NOT NULL DEFAULT '',
  title_preferred TEXT NOT NULL DEFAULT '',
  title_native TEXT NOT NULL DEFAULT '',
  genres_json TEXT NOT NULL DEFAULT '[]',
  episode_maps_json TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_series_enrichment_mal_id ON series_enrichment(mal_id) WHERE mal_id > 0;
CREATE INDEX IF NOT EXISTS idx_series_enrichment_kitsu_anime_id ON series_enrichment(kitsu_anime_id) WHERE kitsu_anime_id <> '';

-- +goose Down
DROP TABLE IF EXISTS series_enrichment;
