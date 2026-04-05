-- +goose Up
CREATE TABLE IF NOT EXISTS series_curation (
  series_id TEXT NOT NULL PRIMARY KEY REFERENCES catalog_series(id) ON DELETE CASCADE,
  last_ai_review_at TIMESTAMPTZ,
  ai_review_status TEXT NOT NULL DEFAULT 'pending',
  canonical_title_suggestion TEXT NOT NULL DEFAULT '',
  season_count_suggestion INTEGER,
  raw_ai_payload_json TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_series_curation_status ON series_curation(ai_review_status);

-- +goose Down
DROP TABLE IF EXISTS series_curation;
