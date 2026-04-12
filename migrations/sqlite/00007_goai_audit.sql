-- +goose Up
CREATE TABLE IF NOT EXISTS goai_series_audit (
  series_id TEXT NOT NULL PRIMARY KEY REFERENCES catalog_series(id) ON DELETE CASCADE,
  audited_at TEXT NOT NULL,
  prompt_version INTEGER NOT NULL DEFAULT 3,
  response_json TEXT NOT NULL DEFAULT '{}',
  needs_reaudit INTEGER NOT NULL DEFAULT 0,
  reaudit_requested_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_goai_series_audit_needs_reaudit ON goai_series_audit(needs_reaudit) WHERE needs_reaudit != 0;

CREATE TABLE IF NOT EXISTS goai_release_audit (
  series_id TEXT NOT NULL REFERENCES catalog_series(id) ON DELETE CASCADE,
  season INTEGER NOT NULL,
  episode INTEGER NOT NULL,
  is_special INTEGER NOT NULL DEFAULT 0,
  audited_at TEXT NOT NULL,
  prompt_version INTEGER NOT NULL DEFAULT 3,
  response_json TEXT NOT NULL DEFAULT '{}',
  source_title TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (series_id, season, episode, is_special)
);

CREATE INDEX IF NOT EXISTS idx_goai_release_audit_series ON goai_release_audit(series_id);

-- +goose Down
DROP TABLE IF EXISTS goai_release_audit;
DROP TABLE IF EXISTS goai_series_audit;
