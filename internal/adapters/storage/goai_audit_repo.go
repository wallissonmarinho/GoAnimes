package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

var _ ports.GoAIAuditRepository = (*catalogRepo)(nil)

func (r *catalogRepo) ListSeriesIDsWithCatalogItems(ctx context.Context) ([]string, error) {
	rows, err := r.ex.QueryContext(ctx, `SELECT DISTINCT series_id FROM catalog_item ORDER BY series_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *catalogRepo) GetSeriesAudit(ctx context.Context, seriesID string) (*domain.GoaiSeriesAuditRecord, error) {
	if r.pg {
		row := r.ex.QueryRowContext(ctx, `
			SELECT series_id, audited_at, prompt_version, response_json, needs_reaudit, reaudit_requested_at
			FROM goai_series_audit WHERE series_id = $1`, seriesID)
		var rec domain.GoaiSeriesAuditRecord
		var reqAt sql.NullTime
		if err := row.Scan(&rec.SeriesID, &rec.AuditedAt, &rec.PromptVersion, &rec.ResponseJSON, &rec.NeedsReaudit, &reqAt); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, err
		}
		if reqAt.Valid {
			t := reqAt.Time
			rec.ReauditRequestedAt = &t
		}
		return &rec, nil
	}
	row := r.ex.QueryRowContext(ctx, `
		SELECT series_id, audited_at, prompt_version, response_json, needs_reaudit, reaudit_requested_at
		FROM goai_series_audit WHERE series_id = ?`, seriesID)
	var rec domain.GoaiSeriesAuditRecord
	var auditedAt, reqAt sql.NullString
	var needsInt int
	if err := row.Scan(&rec.SeriesID, &auditedAt, &rec.PromptVersion, &rec.ResponseJSON, &needsInt, &reqAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if auditedAt.Valid && auditedAt.String != "" {
		t, err := time.Parse(time.RFC3339, auditedAt.String)
		if err != nil {
			return nil, err
		}
		rec.AuditedAt = t
	}
	rec.NeedsReaudit = needsInt != 0
	if reqAt.Valid && reqAt.String != "" {
		t, err := time.Parse(time.RFC3339, reqAt.String)
		if err == nil {
			rec.ReauditRequestedAt = &t
		}
	}
	return &rec, nil
}

func (r *catalogRepo) UpsertSeriesAudit(ctx context.Context, seriesID string, auditedAt time.Time, promptVersion int, responseJSON string, needsReaudit bool) error {
	if r.pg {
		_, err := r.ex.ExecContext(ctx, `
			INSERT INTO goai_series_audit (series_id, audited_at, prompt_version, response_json, needs_reaudit, reaudit_requested_at)
			VALUES ($1,$2,$3,$4,$5,NULL)
			ON CONFLICT (series_id) DO UPDATE SET
				audited_at = EXCLUDED.audited_at,
				prompt_version = EXCLUDED.prompt_version,
				response_json = EXCLUDED.response_json,
				needs_reaudit = EXCLUDED.needs_reaudit,
				reaudit_requested_at = CASE WHEN NOT EXCLUDED.needs_reaudit THEN NULL ELSE goai_series_audit.reaudit_requested_at END`,
			seriesID, auditedAt, promptVersion, responseJSON, needsReaudit)
		return err
	}
	nr := 0
	if needsReaudit {
		nr = 1
	}
	// Preserve reaudit_requested_at when clearing needs_reaudit is awkward in SQLite; admin sets needs via SetSeriesNeedsReaudit.
	_, err := r.ex.ExecContext(ctx, `
		INSERT INTO goai_series_audit (series_id, audited_at, prompt_version, response_json, needs_reaudit, reaudit_requested_at)
		VALUES (?,?,?,?,?,NULL)
		ON CONFLICT(series_id) DO UPDATE SET
			audited_at = excluded.audited_at,
			prompt_version = excluded.prompt_version,
			response_json = excluded.response_json,
			needs_reaudit = excluded.needs_reaudit,
			reaudit_requested_at = CASE WHEN excluded.needs_reaudit = 0 THEN NULL ELSE goai_series_audit.reaudit_requested_at END`,
		seriesID, auditedAt.UTC().Format(time.RFC3339), promptVersion, responseJSON, nr)
	return err
}

func (r *catalogRepo) UpsertReleaseAudit(ctx context.Context, seriesID string, season, episode int, isSpecial bool, auditedAt time.Time, promptVersion int, responseJSON, sourceTitle string) error {
	if r.pg {
		_, err := r.ex.ExecContext(ctx, `
			INSERT INTO goai_release_audit (series_id, season, episode, is_special, audited_at, prompt_version, response_json, source_title)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (series_id, season, episode, is_special) DO UPDATE SET
				audited_at = EXCLUDED.audited_at,
				prompt_version = EXCLUDED.prompt_version,
				response_json = EXCLUDED.response_json,
				source_title = EXCLUDED.source_title`,
			seriesID, season, episode, isSpecial, auditedAt, promptVersion, responseJSON, sourceTitle)
		return err
	}
	is := 0
	if isSpecial {
		is = 1
	}
	_, err := r.ex.ExecContext(ctx, `
		INSERT INTO goai_release_audit (series_id, season, episode, is_special, audited_at, prompt_version, response_json, source_title)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(series_id, season, episode, is_special) DO UPDATE SET
			audited_at = excluded.audited_at,
			prompt_version = excluded.prompt_version,
			response_json = excluded.response_json,
			source_title = excluded.source_title`,
		seriesID, season, episode, is, auditedAt.UTC().Format(time.RFC3339), promptVersion, responseJSON, sourceTitle)
	return err
}

func (r *catalogRepo) ListUnauditedReleaseKeysForSeries(ctx context.Context, seriesID string) ([]domain.GoaiReleaseKey, error) {
	if r.pg {
		rows, err := r.ex.QueryContext(ctx, `
			SELECT DISTINCT ci.season, ci.episode, ci.is_special
			FROM catalog_item ci
			WHERE ci.series_id = $1
			AND NOT EXISTS (
				SELECT 1 FROM goai_release_audit g
				WHERE g.series_id = ci.series_id
				  AND g.season = ci.season
				  AND g.episode = ci.episode
				  AND g.is_special = ci.is_special
				  AND g.prompt_version >= $2
			)
			ORDER BY ci.season ASC, ci.episode ASC`, seriesID, domain.GoaiAuditPromptVersion)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []domain.GoaiReleaseKey
		for rows.Next() {
			var k domain.GoaiReleaseKey
			k.SeriesID = seriesID
			if err := rows.Scan(&k.Season, &k.Episode, &k.IsSpecial); err != nil {
				return nil, err
			}
			out = append(out, k)
		}
		return out, rows.Err()
	}
	rows, err := r.ex.QueryContext(ctx, `
		SELECT DISTINCT ci.season, ci.episode, ci.is_special
		FROM catalog_item ci
		WHERE ci.series_id = ?
		AND NOT EXISTS (
			SELECT 1 FROM goai_release_audit g
			WHERE g.series_id = ci.series_id
			  AND g.season = ci.season
			  AND g.episode = ci.episode
			  AND g.is_special = ci.is_special
			  AND g.prompt_version >= ?
		)
		ORDER BY ci.season ASC, ci.episode ASC`, seriesID, domain.GoaiAuditPromptVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.GoaiReleaseKey
	for rows.Next() {
		var k domain.GoaiReleaseKey
		k.SeriesID = seriesID
		var isInt int
		if err := rows.Scan(&k.Season, &k.Episode, &isInt); err != nil {
			return nil, err
		}
		k.IsSpecial = isInt != 0
		out = append(out, k)
	}
	return out, rows.Err()
}

func (r *catalogRepo) SampleItemTitleForSeries(ctx context.Context, seriesID string) (string, error) {
	ctxItem, err := r.SampleItemContextForSeries(ctx, seriesID)
	if err != nil || ctxItem == nil {
		return "", err
	}
	return ctxItem.Title, nil
}

func (r *catalogRepo) SampleItemTitleForRelease(ctx context.Context, key domain.GoaiReleaseKey) (string, error) {
	ctxItem, err := r.SampleItemContextForRelease(ctx, key)
	if err != nil || ctxItem == nil {
		return "", err
	}
	return ctxItem.Title, nil
}

func (r *catalogRepo) SampleItemContextForSeries(ctx context.Context, seriesID string) (*domain.GoaiAuditItemContext, error) {
	if r.pg {
		row := r.ex.QueryRowContext(ctx, `
			SELECT name, torrent_url, released, season, episode, is_special
			FROM catalog_item
			WHERE series_id = $1
			ORDER BY released DESC NULLS LAST LIMIT 1`, seriesID)
		var out domain.GoaiAuditItemContext
		if err := row.Scan(&out.Title, &out.TorrentURL, &out.Released, &out.Season, &out.Episode, &out.IsSpecial); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, err
		}
		return &out, nil
	}
	row := r.ex.QueryRowContext(ctx, `
		SELECT name, torrent_url, released, season, episode, is_special
		FROM catalog_item
		WHERE series_id = ?
		ORDER BY released DESC LIMIT 1`, seriesID)
	var out domain.GoaiAuditItemContext
	var isInt int
	if err := row.Scan(&out.Title, &out.TorrentURL, &out.Released, &out.Season, &out.Episode, &isInt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	out.IsSpecial = isInt != 0
	return &out, nil
}

func (r *catalogRepo) SampleItemContextForRelease(ctx context.Context, key domain.GoaiReleaseKey) (*domain.GoaiAuditItemContext, error) {
	if r.pg {
		is := key.IsSpecial
		row := r.ex.QueryRowContext(ctx, `
			SELECT name, torrent_url, released, season, episode, is_special FROM catalog_item
			WHERE series_id = $1 AND season = $2 AND episode = $3 AND is_special = $4
			ORDER BY released DESC NULLS LAST LIMIT 1`,
			key.SeriesID, key.Season, key.Episode, is)
		var out domain.GoaiAuditItemContext
		if err := row.Scan(&out.Title, &out.TorrentURL, &out.Released, &out.Season, &out.Episode, &out.IsSpecial); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, err
		}
		return &out, nil
	}
	is := 0
	if key.IsSpecial {
		is = 1
	}
	row := r.ex.QueryRowContext(ctx, `
		SELECT name, torrent_url, released, season, episode, is_special FROM catalog_item
		WHERE series_id = ? AND season = ? AND episode = ? AND is_special = ?
		ORDER BY released DESC LIMIT 1`,
		key.SeriesID, key.Season, key.Episode, is)
	var out domain.GoaiAuditItemContext
	var isInt int
	if err := row.Scan(&out.Title, &out.TorrentURL, &out.Released, &out.Season, &out.Episode, &isInt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	out.IsSpecial = isInt != 0
	return &out, nil
}

func (r *catalogRepo) ListSeriesAuditsForAdmin(ctx context.Context, params domain.GoaiAuditListParams) ([]domain.GoaiSeriesAuditListItem, error) {
	limit := params.Limit
	offset := params.Offset
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	where, args := buildGoaiSeriesAuditConfidenceFilter(r.pg, params.ConfidenceMin, params.ConfidenceMax)
	if r.pg {
		query := `
			SELECT g.series_id, s.name, g.audited_at, g.prompt_version, g.needs_reaudit, g.reaudit_requested_at, g.response_json
			FROM goai_series_audit g
			JOIN catalog_series s ON s.id = g.series_id
		` + where + `
			ORDER BY g.audited_at DESC
			LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2)
		args = append(args, limit, offset)
		rows, err := r.ex.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanGoaiAdminRowsPG(rows)
	}
	query := `
		SELECT g.series_id, s.name, g.audited_at, g.prompt_version, g.needs_reaudit, g.reaudit_requested_at, g.response_json
		FROM goai_series_audit g
		JOIN catalog_series s ON s.id = g.series_id
	` + where + `
		ORDER BY g.audited_at DESC
		LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := r.ex.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGoaiAdminRowsSQLite(rows)
}

func (r *catalogRepo) CountSeriesAuditsForAdmin(ctx context.Context, params domain.GoaiAuditListParams) (int, error) {
	var n int
	where, args := buildGoaiSeriesAuditConfidenceFilter(r.pg, params.ConfidenceMin, params.ConfidenceMax)
	row := r.ex.QueryRowContext(ctx, `SELECT COUNT(*) FROM goai_series_audit g`+where, args...)
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func buildGoaiSeriesAuditConfidenceFilter(pg bool, min, max *float64) (string, []any) {
	if min == nil && max == nil {
		return "", nil
	}
	expr := "COALESCE(CAST(json_extract(g.response_json, '$.confidence') AS REAL), 0.0)"
	if pg {
		expr = "COALESCE((g.response_json::jsonb ->> 'confidence')::double precision, 0.0)"
	}
	args := make([]any, 0, 2)
	clauses := make([]string, 0, 2)
	argPos := 1
	if min != nil {
		if pg {
			clauses = append(clauses, fmt.Sprintf("%s >= $%d", expr, argPos))
			argPos++
		} else {
			clauses = append(clauses, expr+" >= ?")
		}
		args = append(args, *min)
	}
	if max != nil {
		if pg {
			clauses = append(clauses, fmt.Sprintf("%s <= $%d", expr, argPos))
		} else {
			clauses = append(clauses, expr+" <= ?")
		}
		args = append(args, *max)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func scanGoaiAdminRowsPG(rows *sql.Rows) ([]domain.GoaiSeriesAuditListItem, error) {
	var out []domain.GoaiSeriesAuditListItem
	for rows.Next() {
		var it domain.GoaiSeriesAuditListItem
		var reqAt sql.NullTime
		var respJSON string
		if err := rows.Scan(&it.SeriesID, &it.SeriesName, &it.AuditedAt, &it.PromptVersion, &it.NeedsReaudit, &reqAt, &respJSON); err != nil {
			return nil, err
		}
		if reqAt.Valid {
			t := reqAt.Time
			it.ReauditRequestedAt = &t
		}
		applySeriesAuditListFields(&it, respJSON)
		out = append(out, it)
	}
	return out, rows.Err()
}

func scanGoaiAdminRowsSQLite(rows *sql.Rows) ([]domain.GoaiSeriesAuditListItem, error) {
	var out []domain.GoaiSeriesAuditListItem
	for rows.Next() {
		var it domain.GoaiSeriesAuditListItem
		var at, reqAt sql.NullString
		var needs int
		var respJSON string
		if err := rows.Scan(&it.SeriesID, &it.SeriesName, &at, &it.PromptVersion, &needs, &reqAt, &respJSON); err != nil {
			return nil, err
		}
		it.NeedsReaudit = needs != 0
		if at.Valid && at.String != "" {
			t, err := time.Parse(time.RFC3339, at.String)
			if err != nil {
				return nil, err
			}
			it.AuditedAt = t
		}
		if reqAt.Valid && reqAt.String != "" {
			t, err := time.Parse(time.RFC3339, reqAt.String)
			if err == nil {
				it.ReauditRequestedAt = &t
			}
		}
		applySeriesAuditListFields(&it, respJSON)
		out = append(out, it)
	}
	return out, rows.Err()
}

func applySeriesAuditListFields(it *domain.GoaiSeriesAuditListItem, respJSON string) {
	respJSON = strings.TrimSpace(respJSON)
	if respJSON == "" {
		return
	}
	it.RawResponseJSON = respJSON
	var resp domain.GoaiSeriesAuditResponse
	if err := json.Unmarshal([]byte(respJSON), &resp); err != nil {
		return
	}
	it.TheTVDBSeriesID = resp.TheTVDBSeriesID
	it.TheTVDBSeriesURL = resp.TheTVDBSeriesURL
	it.TheTVDBName = resp.TheTVDBName
	it.MalID = resp.MalID
	it.AniDBAID = resp.AniDBAID
	it.AniListID = resp.AniListID
	it.TMDBTVID = resp.TMDBTVID
	it.ReleaseSeason = resp.ReleaseSeason
	it.ReleaseEpisode = resp.ReleaseEpisode
	it.ReleaseIsSpecial = resp.ReleaseIsSpecial
	it.Confidence = resp.Confidence
	it.Notes = resp.Notes
}

func (r *catalogRepo) DeleteReleaseAuditsForSeries(ctx context.Context, seriesID string) error {
	if r.pg {
		_, err := r.ex.ExecContext(ctx, `DELETE FROM goai_release_audit WHERE series_id = $1`, seriesID)
		return err
	}
	_, err := r.ex.ExecContext(ctx, `DELETE FROM goai_release_audit WHERE series_id = ?`, seriesID)
	return err
}

func (r *catalogRepo) SetSeriesNeedsReaudit(ctx context.Context, seriesID string) error {
	now := time.Now().UTC()
	if r.pg {
		res, err := r.ex.ExecContext(ctx, `
			UPDATE goai_series_audit SET needs_reaudit = TRUE, reaudit_requested_at = $1 WHERE series_id = $2`,
			now, seriesID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return sql.ErrNoRows
		}
		return nil
	}
	res, err := r.ex.ExecContext(ctx, `
		UPDATE goai_series_audit SET needs_reaudit = 1, reaudit_requested_at = ? WHERE series_id = ?`,
		now.Format(time.RFC3339), seriesID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *catalogRepo) UpdateSeriesEnrichmentTVDB(ctx context.Context, seriesID string, tvdbSeriesID int) error {
	if tvdbSeriesID <= 0 {
		return nil
	}
	if r.pg {
		_, err := r.ex.ExecContext(ctx, `UPDATE series_enrichment SET tvdb_series_id = $1 WHERE series_id = $2`, tvdbSeriesID, seriesID)
		return err
	}
	_, err := r.ex.ExecContext(ctx, `UPDATE series_enrichment SET tvdb_series_id = ? WHERE series_id = ?`, tvdbSeriesID, seriesID)
	return err
}

// GoaiAuditRepo returns a GoAIAuditRepository backed by this catalog DB.
func (c *Catalog) GoaiAuditRepo() ports.GoAIAuditRepository {
	return &catalogRepo{ex: c.db, pg: c.pg}
}

func (r *catalogRepo) GetCatalogSeriesName(ctx context.Context, seriesID string) (string, error) {
	if r.pg {
		row := r.ex.QueryRowContext(ctx, `SELECT name FROM catalog_series WHERE id = $1`, seriesID)
		var name string
		if err := row.Scan(&name); err != nil {
			if err == sql.ErrNoRows {
				return "", nil
			}
			return "", err
		}
		return name, nil
	}
	row := r.ex.QueryRowContext(ctx, `SELECT name FROM catalog_series WHERE id = ?`, seriesID)
	var name string
	if err := row.Scan(&name); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return name, nil
}

func (r *catalogRepo) GetEnrichmentTVDBSeriesID(ctx context.Context, seriesID string) (int, error) {
	if r.pg {
		row := r.ex.QueryRowContext(ctx, `SELECT tvdb_series_id FROM series_enrichment WHERE series_id = $1`, seriesID)
		var tvdb int
		if err := row.Scan(&tvdb); err != nil {
			if err == sql.ErrNoRows {
				return 0, nil
			}
			return 0, err
		}
		return tvdb, nil
	}
	row := r.ex.QueryRowContext(ctx, `SELECT tvdb_series_id FROM series_enrichment WHERE series_id = ?`, seriesID)
	var tvdb int
	if err := row.Scan(&tvdb); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return tvdb, nil
}
