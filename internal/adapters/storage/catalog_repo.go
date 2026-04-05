package storage

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type catalogRepo struct {
	ex sqlExecutor
	pg bool
}

func (r *catalogRepo) FindRSSSourceByURL(ctx context.Context, url string) (*domain.RSSSource, error) {
	var row *sql.Row
	if r.pg {
		row = r.ex.QueryRowContext(ctx,
			`SELECT id, url, label, created_at FROM rss_sources WHERE url = $1`, url)
	} else {
		row = r.ex.QueryRowContext(ctx,
			`SELECT id, url, label, created_at FROM rss_sources WHERE url = ?`, url)
	}
	var s domain.RSSSource
	if r.pg {
		if err := row.Scan(&s.ID, &s.URL, &s.Label, &s.CreatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil
			}
			return nil, err
		}
		return &s, nil
	}
	var created string
	if err := row.Scan(&s.ID, &s.URL, &s.Label, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return &s, nil
}

func (r *catalogRepo) CreateRSSSource(ctx context.Context, url, label string) (*domain.RSSSource, error) {
	now := time.Now().UTC()
	id := uuid.New().String()
	if r.pg {
		_, err := r.ex.ExecContext(ctx,
			`INSERT INTO rss_sources (id, url, label, created_at) VALUES ($1,$2,$3,$4)`,
			id, url, label, now)
		if err != nil {
			return nil, err
		}
		return &domain.RSSSource{ID: id, URL: url, Label: label, CreatedAt: now}, nil
	}
	_, err := r.ex.ExecContext(ctx,
		`INSERT INTO rss_sources (id, url, label, created_at) VALUES (?,?,?,?)`,
		id, url, label, now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return &domain.RSSSource{ID: id, URL: url, Label: label, CreatedAt: now}, nil
}

func (r *catalogRepo) ListRSSSources(ctx context.Context) ([]domain.RSSSource, error) {
	rows, err := r.ex.QueryContext(ctx, `SELECT id, url, label, created_at FROM rss_sources ORDER BY created_at ASC, url ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.RSSSource
	for rows.Next() {
		var s domain.RSSSource
		if r.pg {
			if err := rows.Scan(&s.ID, &s.URL, &s.Label, &s.CreatedAt); err != nil {
				return nil, err
			}
		} else {
			var created string
			if err := rows.Scan(&s.ID, &s.URL, &s.Label, &created); err != nil {
				return nil, err
			}
			s.CreatedAt, _ = time.Parse(time.RFC3339, created)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *catalogRepo) DeleteRSSSource(ctx context.Context, id string) error {
	var res sql.Result
	var err error
	if r.pg {
		res, err = r.ex.ExecContext(ctx, `DELETE FROM rss_sources WHERE id = $1`, id)
	} else {
		res, err = r.ex.ExecContext(ctx, `DELETE FROM rss_sources WHERE id = ?`, id)
	}
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type catalogPayload struct {
	Items            []domain.CatalogItem                          `json:"items"`
	LastSyncErrors   []string                                      `json:"last_sync_errors,omitempty"`
	AniListPosters   map[string]string                             `json:"anilist_posters,omitempty"` // legacy: poster URL only
	AniListSeries    map[string]domain.AniListSeriesEnrichment     `json:"anilist_series,omitempty"`
	RSSMainFeedBuild map[string]domain.RssMainFeedBuildFingerprint `json:"rss_main_feed_build,omitempty"`
}

func marshalCatalogPayload(snap domain.CatalogSnapshot, omitAniListSeries bool) ([]byte, error) {
	p := catalogPayload{
		Items:            snap.Items,
		LastSyncErrors:   snap.LastSyncErrors,
		RSSMainFeedBuild: snap.RSSMainFeedBuildByURL,
	}
	if !omitAniListSeries {
		p.AniListSeries = snap.AniListBySeries
	}
	if len(p.LastSyncErrors) == 0 {
		p.LastSyncErrors = nil
	}
	if len(p.AniListSeries) == 0 {
		p.AniListSeries = nil
	}
	if len(p.RSSMainFeedBuild) == 0 {
		p.RSSMainFeedBuild = nil
	}
	return json.Marshal(p)
}

func unmarshalCatalogPayload(raw []byte) (domain.CatalogSnapshot, error) {
	var snap domain.CatalogSnapshot
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return snap, nil
	}
	if raw[0] == '[' {
		if err := json.Unmarshal(raw, &snap.Items); err != nil {
			return domain.CatalogSnapshot{}, err
		}
		snap.AniListBySeries = make(map[string]domain.AniListSeriesEnrichment)
		snap.RSSMainFeedBuildByURL = make(map[string]domain.RssMainFeedBuildFingerprint)
		return snap, nil
	}
	var p catalogPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return domain.CatalogSnapshot{}, err
	}
	snap.Items = p.Items
	snap.LastSyncErrors = append([]string(nil), p.LastSyncErrors...)
	snap.AniListBySeries = p.AniListSeries
	snap.RSSMainFeedBuildByURL = p.RSSMainFeedBuild
	if snap.AniListBySeries == nil {
		snap.AniListBySeries = make(map[string]domain.AniListSeriesEnrichment)
	}
	if snap.RSSMainFeedBuildByURL == nil {
		snap.RSSMainFeedBuildByURL = make(map[string]domain.RssMainFeedBuildFingerprint)
	}
	// Legacy snapshot: only anilist_posters map.
	for k, url := range p.AniListPosters {
		url = strings.TrimSpace(url)
		if url == "" {
			continue
		}
		e := snap.AniListBySeries[k]
		if e.PosterURL == "" {
			e.PosterURL = url
		}
		snap.AniListBySeries[k] = e
	}
	return snap, nil
}

func (r *catalogRepo) persistCatalogSnapshotRow(ctx context.Context, snap domain.CatalogSnapshot, omitAniListSeries bool) error {
	b, err := marshalCatalogPayload(snap, omitAniListSeries)
	if err != nil {
		return err
	}
	if r.pg {
		_, err = r.ex.ExecContext(ctx, `
			UPDATE catalog_snapshot SET
				items_json = $1,
				ok = $2,
				message = $3,
				item_count = $4,
				started_at = $5,
				finished_at = $6
			WHERE id = 1`,
			b, snap.OK, snap.Message, snap.ItemCount,
			nullTimePG(snap.StartedAt), nullTimePG(snap.FinishedAt))
		return err
	}
	started := ""
	if !snap.StartedAt.IsZero() {
		started = snap.StartedAt.UTC().Format(time.RFC3339)
	}
	finished := ""
	if !snap.FinishedAt.IsZero() {
		finished = snap.FinishedAt.UTC().Format(time.RFC3339)
	}
	_, err = r.ex.ExecContext(ctx, `
		UPDATE catalog_snapshot SET
			items_json = ?,
			ok = ?,
			message = ?,
			item_count = ?,
			started_at = ?,
			finished_at = ?
		WHERE id = 1`,
		string(b), boolToInt(snap.OK), snap.Message, snap.ItemCount, started, finished)
	return err
}

func (r *catalogRepo) LoadCatalogSnapshot(ctx context.Context) (domain.CatalogSnapshot, error) {
	snap, err := r.loadCatalogSnapshotRow(ctx)
	if err != nil {
		return snap, err
	}
	return r.mergeNormalizedIntoSnapshot(ctx, snap)
}

// loadCatalogSnapshotRow reads catalog_snapshot and unmarshals items_json only (no normalized tables).
func (r *catalogRepo) loadCatalogSnapshotRow(ctx context.Context) (domain.CatalogSnapshot, error) {
	var (
		rawItems []byte
		msg      string
		itemCnt  int
		ok       bool
		started  sql.NullTime
		finished sql.NullTime
	)
	if r.pg {
		row := r.ex.QueryRowContext(ctx, `
			SELECT items_json, ok, message, item_count, started_at, finished_at
			FROM catalog_snapshot WHERE id = 1`)
		if err := row.Scan(&rawItems, &ok, &msg, &itemCnt, &started, &finished); err != nil {
			return domain.CatalogSnapshot{}, err
		}
	} else {
		var okInt int
		var st, ft sql.NullString
		row := r.ex.QueryRowContext(ctx, `
			SELECT items_json, ok, message, item_count, started_at, finished_at
			FROM catalog_snapshot WHERE id = 1`)
		if err := row.Scan(&rawItems, &okInt, &msg, &itemCnt, &st, &ft); err != nil {
			return domain.CatalogSnapshot{}, err
		}
		ok = okInt != 0
		if st.Valid {
			t, _ := time.Parse(time.RFC3339, st.String)
			started = sql.NullTime{Time: t, Valid: true}
		}
		if ft.Valid {
			t, _ := time.Parse(time.RFC3339, ft.String)
			finished = sql.NullTime{Time: t, Valid: true}
		}
	}
	snap, err := unmarshalCatalogPayload(rawItems)
	if err != nil {
		return domain.CatalogSnapshot{}, err
	}
	snap.OK = ok
	snap.Message = msg
	snap.ItemCount = itemCnt
	if started.Valid {
		snap.StartedAt = started.Time
	}
	if finished.Valid {
		snap.FinishedAt = finished.Time
	}
	return snap, nil
}

func nullTimePG(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
