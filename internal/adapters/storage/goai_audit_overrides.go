package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

type goaiReleaseKey struct {
	SeriesID  string
	Season    int
	Episode   int
	IsSpecial bool
}

func goaiSourceKey(seriesID, sourceTitle string) string {
	return strings.TrimSpace(seriesID) + "\x1f" + strings.TrimSpace(strings.ToLower(sourceTitle))
}

func validReleaseOverride(resp domain.GoaiReleaseAuditResponse) bool {
	// Keep this conservative: only apply when GoAI returned concrete episode coordinates.
	return resp.Season > 0 && resp.Episode > 0
}

// ApplyGoaiReleaseAuditOverrides rewrites item season/episode/is_special using persisted GoAI release audit.
// If no audit row matches, items stay unchanged.
func (c *Catalog) ApplyGoaiReleaseAuditOverrides(ctx context.Context, items []domain.CatalogItem) ([]domain.CatalogItem, error) {
	return (&catalogRepo{ex: c.db, pg: c.pg}).ApplyGoaiReleaseAuditOverrides(ctx, items)
}

func (r *catalogRepo) ApplyGoaiReleaseAuditOverrides(ctx context.Context, items []domain.CatalogItem) ([]domain.CatalogItem, error) {
	if len(items) == 0 {
		return items, nil
	}
	byKey := make(map[goaiReleaseKey]domain.GoaiReleaseAuditResponse, len(items))
	bySource := make(map[string]domain.GoaiReleaseAuditResponse, len(items))

	if r.pg {
		rows, err := r.ex.QueryContext(ctx, `
			SELECT series_id, season, episode, is_special, audited_at, response_json, source_title
			FROM goai_release_audit
			ORDER BY audited_at DESC`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				seriesID  string
				season    int
				episode   int
				isSpecial bool
				auditedAt time.Time
				respJSON  string
				source    string
			)
			if err := rows.Scan(&seriesID, &season, &episode, &isSpecial, &auditedAt, &respJSON, &source); err != nil {
				return nil, err
			}
			var resp domain.GoaiReleaseAuditResponse
			if err := json.Unmarshal([]byte(respJSON), &resp); err != nil || !validReleaseOverride(resp) {
				continue
			}
			k := goaiReleaseKey{SeriesID: seriesID, Season: season, Episode: episode, IsSpecial: isSpecial}
			if _, ok := byKey[k]; !ok {
				byKey[k] = resp
			}
			sk := goaiSourceKey(seriesID, source)
			if sk != "" && sk != "\x1f" {
				if _, ok := bySource[sk]; !ok {
					bySource[sk] = resp
				}
			}
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	} else {
		rows, err := r.ex.QueryContext(ctx, `
			SELECT series_id, season, episode, is_special, audited_at, response_json, source_title
			FROM goai_release_audit
			ORDER BY audited_at DESC`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				seriesID string
				season   int
				episode  int
				isInt    int
				atRaw    sql.NullString
				respJSON string
				source   string
			)
			if err := rows.Scan(&seriesID, &season, &episode, &isInt, &atRaw, &respJSON, &source); err != nil {
				return nil, err
			}
			var resp domain.GoaiReleaseAuditResponse
			if err := json.Unmarshal([]byte(respJSON), &resp); err != nil || !validReleaseOverride(resp) {
				continue
			}
			k := goaiReleaseKey{SeriesID: seriesID, Season: season, Episode: episode, IsSpecial: isInt != 0}
			if _, ok := byKey[k]; !ok {
				byKey[k] = resp
			}
			sk := goaiSourceKey(seriesID, source)
			if sk != "" && sk != "\x1f" {
				if _, ok := bySource[sk]; !ok {
					bySource[sk] = resp
				}
			}
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	out := make([]domain.CatalogItem, len(items))
	copy(out, items)
	for i := range out {
		it := out[i]
		key := goaiReleaseKey{SeriesID: it.SeriesID, Season: it.Season, Episode: it.Episode, IsSpecial: it.IsSpecial}
		if resp, ok := byKey[key]; ok {
			out[i].Season = resp.Season
			out[i].Episode = resp.Episode
			out[i].IsSpecial = resp.IsSpecial
			continue
		}
		if resp, ok := bySource[goaiSourceKey(it.SeriesID, it.Name)]; ok {
			out[i].Season = resp.Season
			out[i].Episode = resp.Episode
			out[i].IsSpecial = resp.IsSpecial
		}
	}
	return out, nil
}
