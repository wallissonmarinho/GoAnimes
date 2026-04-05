package storage

import (
	"context"
	"encoding/json"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

func (r *catalogRepo) countCatalogItems(ctx context.Context) (int64, error) {
	row := r.ex.QueryRowContext(ctx, `SELECT COUNT(*) FROM catalog_item`)
	var n int64
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (r *catalogRepo) replaceNormalizedCatalog(ctx context.Context, snap domain.CatalogSnapshot) error {
	if _, err := r.ex.ExecContext(ctx, `DELETE FROM catalog_item`); err != nil {
		return err
	}
	if _, err := r.ex.ExecContext(ctx, `DELETE FROM catalog_series`); err != nil {
		return err
	}
	for _, s := range snap.Series {
		gb, err := json.Marshal(s.Genres)
		if err != nil {
			return err
		}
		if r.pg {
			_, err = r.ex.ExecContext(ctx,
				`INSERT INTO catalog_series (id, name, poster, description, genres_json, release_info) VALUES ($1,$2,$3,$4,$5,$6)`,
				s.ID, s.Name, s.Poster, s.Description, string(gb), s.ReleaseInfo)
		} else {
			_, err = r.ex.ExecContext(ctx,
				`INSERT INTO catalog_series (id, name, poster, description, genres_json, release_info) VALUES (?,?,?,?,?,?)`,
				s.ID, s.Name, s.Poster, s.Description, string(gb), s.ReleaseInfo)
		}
		if err != nil {
			return err
		}
	}
	if err := r.insertAllSeriesEnrichments(ctx, snap); err != nil {
		return err
	}
	for _, it := range snap.Items {
		if r.pg {
			_, err := r.ex.ExecContext(ctx, `
				INSERT INTO catalog_item (
					id, series_id, type, name, poster, magnet_url, torrent_url, info_hash,
					released, subtitles_tag, series_name, season, episode, is_special
				) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
				it.ID, it.SeriesID, it.Type, it.Name, it.Poster, it.MagnetURL, it.TorrentURL, it.InfoHash,
				it.Released, it.SubtitlesTag, it.SeriesName, it.Season, it.Episode, it.IsSpecial)
			if err != nil {
				return err
			}
		} else {
			isSpec := 0
			if it.IsSpecial {
				isSpec = 1
			}
			_, err := r.ex.ExecContext(ctx, `
				INSERT INTO catalog_item (
					id, series_id, type, name, poster, magnet_url, torrent_url, info_hash,
					released, subtitles_tag, series_name, season, episode, is_special
				) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				it.ID, it.SeriesID, it.Type, it.Name, it.Poster, it.MagnetURL, it.TorrentURL, it.InfoHash,
				it.Released, it.SubtitlesTag, it.SeriesName, it.Season, it.Episode, isSpec)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *catalogRepo) loadNormalizedCatalog(ctx context.Context) ([]domain.CatalogSeries, []domain.CatalogItem, error) {
	rows, err := r.ex.QueryContext(ctx,
		`SELECT id, name, poster, description, genres_json, release_info FROM catalog_series ORDER BY name ASC`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var series []domain.CatalogSeries
	for rows.Next() {
		var s domain.CatalogSeries
		var gj string
		if err := rows.Scan(&s.ID, &s.Name, &s.Poster, &s.Description, &gj, &s.ReleaseInfo); err != nil {
			return nil, nil, err
		}
		_ = json.Unmarshal([]byte(gj), &s.Genres)
		series = append(series, s)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	irows, err := r.ex.QueryContext(ctx, `
		SELECT id, series_id, type, name, poster, magnet_url, torrent_url, info_hash,
		       released, subtitles_tag, series_name, season, episode, is_special
		FROM catalog_item ORDER BY series_id ASC, season ASC, episode ASC`)
	if err != nil {
		return nil, nil, err
	}
	defer irows.Close()
	var items []domain.CatalogItem
	for irows.Next() {
		var it domain.CatalogItem
		if r.pg {
			if err := irows.Scan(&it.ID, &it.SeriesID, &it.Type, &it.Name, &it.Poster, &it.MagnetURL, &it.TorrentURL, &it.InfoHash,
				&it.Released, &it.SubtitlesTag, &it.SeriesName, &it.Season, &it.Episode, &it.IsSpecial); err != nil {
				return nil, nil, err
			}
		} else {
			var isSpec int
			if err := irows.Scan(&it.ID, &it.SeriesID, &it.Type, &it.Name, &it.Poster, &it.MagnetURL, &it.TorrentURL, &it.InfoHash,
				&it.Released, &it.SubtitlesTag, &it.SeriesName, &it.Season, &it.Episode, &isSpec); err != nil {
				return nil, nil, err
			}
			it.IsSpecial = isSpec != 0
		}
		items = append(items, it)
	}
	return series, items, irows.Err()
}

func (r *catalogRepo) saveCatalogSnapshotAndNormalized(ctx context.Context, snap domain.CatalogSnapshot) error {
	s := snap
	domain.EnsureSnapshotGrouped(&s)
	if err := r.replaceNormalizedCatalog(ctx, s); err != nil {
		return err
	}
	ce, err := r.countSeriesEnrichment(ctx)
	if err != nil {
		return err
	}
	return r.persistCatalogSnapshotRow(ctx, s, ce > 0)
}

// SaveCatalogSnapshot writes items_json and replaces catalog_series / catalog_item atomically when ex is a transaction.
func (r *catalogRepo) SaveCatalogSnapshot(ctx context.Context, snap domain.CatalogSnapshot) error {
	return r.saveCatalogSnapshotAndNormalized(ctx, snap)
}

func (r *catalogRepo) mergeNormalizedIntoSnapshot(ctx context.Context, snap domain.CatalogSnapshot) (domain.CatalogSnapshot, error) {
	n, err := r.countCatalogItems(ctx)
	if err != nil {
		return snap, err
	}
	if n == 0 {
		return snap, nil
	}
	ser, it, err := r.loadNormalizedCatalog(ctx)
	if err != nil {
		return snap, err
	}
	snap.Items = it
	snap.Series = ser
	snap.ItemCount = len(it)

	ce, err := r.countSeriesEnrichment(ctx)
	if err != nil {
		return snap, err
	}
	if ce > 0 {
		m, err := r.loadAllSeriesEnrichments(ctx)
		if err != nil {
			return snap, err
		}
		snap.AniListBySeries = m
	} else if len(snap.AniListBySeries) > 0 {
		if err := r.backfillSeriesEnrichmentFromSnapshot(ctx, &snap); err != nil {
			return snap, err
		}
		m, err := r.loadAllSeriesEnrichments(ctx)
		if err != nil {
			return snap, err
		}
		snap.AniListBySeries = m
	}
	return snap, nil
}
