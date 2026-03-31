package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

func (r *catalogRepo) SaveCatalogSnapshot(ctx context.Context, snap domain.CatalogSnapshot) error {
	b, err := json.Marshal(snap.Items)
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
	var items []domain.CatalogItem
	if len(rawItems) > 0 {
		_ = json.Unmarshal(rawItems, &items)
	}
	snap := domain.CatalogSnapshot{
		OK:        ok,
		Message:   msg,
		ItemCount: itemCnt,
		Items:     items,
	}
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
