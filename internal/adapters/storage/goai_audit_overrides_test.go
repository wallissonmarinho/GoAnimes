package storage

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func setupGoaiReleaseAuditTable(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		CREATE TABLE goai_release_audit (
			series_id TEXT NOT NULL,
			season INTEGER NOT NULL,
			episode INTEGER NOT NULL,
			is_special INTEGER NOT NULL DEFAULT 0,
			audited_at TEXT NOT NULL,
			prompt_version INTEGER NOT NULL DEFAULT 3,
			response_json TEXT NOT NULL DEFAULT '{}',
			source_title TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (series_id, season, episode, is_special)
		);
	`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
}

func TestApplyGoaiReleaseAuditOverrides_KeyMatch(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	setupGoaiReleaseAuditTable(t, db)

	_, err = db.Exec(
		`INSERT INTO goai_release_audit(series_id, season, episode, is_special, audited_at, prompt_version, response_json, source_title)
		 VALUES (?,?,?,?,?,?,?,?)`,
		"s1", 1, 2, 0, time.Now().UTC().Format(time.RFC3339), 3,
		`{"season":4,"episode":2,"is_special":false,"confidence":0.9}`,
		"Honzuki S1E2")
	if err != nil {
		t.Fatal(err)
	}

	r := &catalogRepo{ex: db, pg: false}
	out, err := r.ApplyGoaiReleaseAuditOverrides(context.Background(), []domain.CatalogItem{
		{SeriesID: "s1", Name: "Honzuki S1E2", Season: 1, Episode: 2, IsSpecial: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Season != 4 || out[0].Episode != 2 || out[0].IsSpecial {
		t.Fatalf("unexpected override: %+v", out[0])
	}
}

func TestApplyGoaiReleaseAuditOverrides_SourceTitleFallback(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	setupGoaiReleaseAuditTable(t, db)

	// Simulate old audits saved by corrected key; source_title fallback should still remap current parsed key.
	_, err = db.Exec(
		`INSERT INTO goai_release_audit(series_id, season, episode, is_special, audited_at, prompt_version, response_json, source_title)
		 VALUES (?,?,?,?,?,?,?,?)`,
		"s1", 4, 2, 0, time.Now().UTC().Format(time.RFC3339), 3,
		`{"season":4,"episode":2,"is_special":false,"confidence":0.9}`,
		"Honzuki raw title")
	if err != nil {
		t.Fatal(err)
	}

	r := &catalogRepo{ex: db, pg: false}
	out, err := r.ApplyGoaiReleaseAuditOverrides(context.Background(), []domain.CatalogItem{
		{SeriesID: "s1", Name: "Honzuki raw title", Season: 1, Episode: 2, IsSpecial: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Season != 4 || out[0].Episode != 2 || out[0].IsSpecial {
		t.Fatalf("unexpected override: %+v", out[0])
	}
}
