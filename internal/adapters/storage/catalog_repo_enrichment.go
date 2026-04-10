package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

// episodeMapsJSON mirrors episode_maps_json in DB (string keys for episode numbers).
type episodeMapsJSON struct {
	EpTitles map[string]string `json:"ep_titles,omitempty"`
	EpThumbs map[string]string `json:"ep_thumbs,omitempty"`
}

func enrichmentEpisodeMapsJSON(en domain.AniListSeriesEnrichment) (string, error) {
	p := episodeMapsJSON{EpTitles: map[string]string{}, EpThumbs: map[string]string{}}
	for k, v := range en.EpisodeTitleByNum {
		p.EpTitles[strconv.Itoa(k)] = v
	}
	for k, v := range en.EpisodeThumbnailByNum {
		p.EpThumbs[strconv.Itoa(k)] = v
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "{}", err
	}
	return string(b), nil
}

func enrichmentFromEpisodeMapsJSON(raw string, en *domain.AniListSeriesEnrichment) error {
	raw = trimJSON(raw)
	if raw == "" || raw == "{}" {
		en.EpisodeTitleByNum = map[int]string{}
		en.EpisodeThumbnailByNum = nil
		return nil
	}
	var p episodeMapsJSON
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return err
	}
	en.EpisodeTitleByNum = map[int]string{}
	for ks, v := range p.EpTitles {
		n, err := strconv.Atoi(ks)
		if err != nil {
			continue
		}
		en.EpisodeTitleByNum[n] = v
	}
	if len(p.EpThumbs) > 0 {
		en.EpisodeThumbnailByNum = make(map[int]string)
		for ks, v := range p.EpThumbs {
			n, err := strconv.Atoi(ks)
			if err != nil {
				continue
			}
			en.EpisodeThumbnailByNum[n] = v
		}
	}
	return nil
}

func trimJSON(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t') {
		s = s[1:]
	}
	return s
}

func (r *catalogRepo) countSeriesEnrichment(ctx context.Context) (int64, error) {
	row := r.ex.QueryRowContext(ctx, `SELECT COUNT(*) FROM series_enrichment`)
	var n int64
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (r *catalogRepo) insertSeriesEnrichmentRow(ctx context.Context, seriesID string, en domain.AniListSeriesEnrichment) error {
	gb, err := json.Marshal(en.Genres)
	if err != nil {
		return err
	}
	em, err := enrichmentEpisodeMapsJSON(en)
	if err != nil {
		return err
	}
	if r.pg {
		_, err = r.ex.ExecContext(ctx, `
			INSERT INTO series_enrichment (
				series_id, anilist_media_id, mal_id, imdb_id, kitsu_anime_id, tvdb_series_id, anidb_aid, anidb_last_fetch_unix,
				al_search_ver, next_air_unix, next_air_ep, next_air_from_al, start_year, episode_length_min,
				poster_url, background_url, al_banner_url, hero_bg_url, description, trailer_youtube_id,
				title_preferred, title_native, genres_json, episode_maps_json
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)`,
			seriesID, nil, en.MalID, en.ImdbID, en.KitsuAnimeID, en.TvdbSeriesID, en.AniDBAid, en.AniDBLastFetchedUnix,
			en.AniListSearchVer, en.NextAiringUnix, en.NextAiringEpisode, en.NextAiringFromAniList, en.StartYear, en.EpisodeLengthMin,
			en.PosterURL, en.BackgroundURL, en.AniListBannerURL, en.StremioHeroBackgroundURL, en.Description, en.TrailerYouTubeID,
			en.TitlePreferred, en.TitleNative, string(gb), em)
		return err
	}
	nextFromAL := 0
	if en.NextAiringFromAniList {
		nextFromAL = 1
	}
	_, err = r.ex.ExecContext(ctx, `
		INSERT INTO series_enrichment (
			series_id, anilist_media_id, mal_id, imdb_id, kitsu_anime_id, tvdb_series_id, anidb_aid, anidb_last_fetch_unix,
			al_search_ver, next_air_unix, next_air_ep, next_air_from_al, start_year, episode_length_min,
			poster_url, background_url, al_banner_url, hero_bg_url, description, trailer_youtube_id,
			title_preferred, title_native, genres_json, episode_maps_json
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		seriesID, nil, en.MalID, en.ImdbID, en.KitsuAnimeID, en.TvdbSeriesID, en.AniDBAid, en.AniDBLastFetchedUnix,
		en.AniListSearchVer, en.NextAiringUnix, en.NextAiringEpisode, nextFromAL, en.StartYear, en.EpisodeLengthMin,
		en.PosterURL, en.BackgroundURL, en.AniListBannerURL, en.StremioHeroBackgroundURL, en.Description, en.TrailerYouTubeID,
		en.TitlePreferred, en.TitleNative, string(gb), em)
	return err
}

func (r *catalogRepo) insertAllSeriesEnrichments(ctx context.Context, snap domain.CatalogSnapshot) error {
	if len(snap.AniListBySeries) == 0 {
		return nil
	}
	seriesIDs := make(map[string]struct{}, len(snap.Series))
	for _, s := range snap.Series {
		seriesIDs[s.ID] = struct{}{}
	}
	for sid, en := range snap.AniListBySeries {
		if _, ok := seriesIDs[sid]; !ok {
			continue
		}
		if err := r.insertSeriesEnrichmentRow(ctx, sid, en); err != nil {
			return err
		}
	}
	return nil
}

func (r *catalogRepo) loadAllSeriesEnrichments(ctx context.Context) (map[string]domain.AniListSeriesEnrichment, error) {
	rows, err := r.ex.QueryContext(ctx, `
		SELECT series_id, anilist_media_id, mal_id, imdb_id, kitsu_anime_id, tvdb_series_id, anidb_aid, anidb_last_fetch_unix,
		       al_search_ver, next_air_unix, next_air_ep, next_air_from_al, start_year, episode_length_min,
		       poster_url, background_url, al_banner_url, hero_bg_url, description, trailer_youtube_id,
		       title_preferred, title_native, genres_json, episode_maps_json
		FROM series_enrichment`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]domain.AniListSeriesEnrichment)
	for rows.Next() {
		var (
			sid, imdb, kitsu, poster, bg, alBanner, hero, desc, trailer, titlePref, titleNat, gj, em string
			mal, tvdb, anidb, alVer, nextEp, startY, epLen                                            int
			anidbFetch, nextUnix                                                                       int64
			alMedia                                                                                    sql.NullInt64
		)
		if r.pg {
			var nextFromAL bool
			if err := rows.Scan(&sid, &alMedia, &mal, &imdb, &kitsu, &tvdb, &anidb, &anidbFetch,
				&alVer, &nextUnix, &nextEp, &nextFromAL, &startY, &epLen,
				&poster, &bg, &alBanner, &hero, &desc, &trailer, &titlePref, &titleNat, &gj, &em); err != nil {
				return nil, err
			}
			en := domain.AniListSeriesEnrichment{
				PosterURL: poster, BackgroundURL: bg, AniListBannerURL: alBanner, StremioHeroBackgroundURL: hero,
				Description: desc, MalID: mal, ImdbID: imdb, KitsuAnimeID: kitsu, TvdbSeriesID: tvdb, AniDBAid: anidb,
				AniDBLastFetchedUnix: anidbFetch, AniListSearchVer: alVer, NextAiringUnix: nextUnix, NextAiringEpisode: nextEp,
				NextAiringFromAniList: nextFromAL, StartYear: startY, EpisodeLengthMin: epLen, TrailerYouTubeID: trailer,
				TitlePreferred: titlePref, TitleNative: titleNat,
			}
			_ = alMedia
			_ = json.Unmarshal([]byte(gj), &en.Genres)
			if err := enrichmentFromEpisodeMapsJSON(em, &en); err != nil {
				en.EpisodeTitleByNum = map[int]string{}
			}
			out[sid] = en
		} else {
			var nextALInt int
			if err := rows.Scan(&sid, &alMedia, &mal, &imdb, &kitsu, &tvdb, &anidb, &anidbFetch,
				&alVer, &nextUnix, &nextEp, &nextALInt, &startY, &epLen,
				&poster, &bg, &alBanner, &hero, &desc, &trailer, &titlePref, &titleNat, &gj, &em); err != nil {
				return nil, err
			}
			en := domain.AniListSeriesEnrichment{
				PosterURL: poster, BackgroundURL: bg, AniListBannerURL: alBanner, StremioHeroBackgroundURL: hero,
				Description: desc, MalID: mal, ImdbID: imdb, KitsuAnimeID: kitsu, TvdbSeriesID: tvdb, AniDBAid: anidb,
				AniDBLastFetchedUnix: anidbFetch, AniListSearchVer: alVer, NextAiringUnix: nextUnix, NextAiringEpisode: nextEp,
				NextAiringFromAniList: nextALInt != 0, StartYear: startY, EpisodeLengthMin: epLen, TrailerYouTubeID: trailer,
				TitlePreferred: titlePref, TitleNative: titleNat,
			}
			_ = alMedia
			_ = json.Unmarshal([]byte(gj), &en.Genres)
			if err := enrichmentFromEpisodeMapsJSON(em, &en); err != nil {
				en.EpisodeTitleByNum = map[int]string{}
			}
			out[sid] = en
		}
	}
	return out, rows.Err()
}

func (r *catalogRepo) backfillSeriesEnrichmentFromSnapshot(ctx context.Context, snap *domain.CatalogSnapshot) error {
	if len(snap.AniListBySeries) == 0 {
		return nil
	}
	for _, s := range snap.Series {
		en, ok := snap.AniListBySeries[s.ID]
		if !ok {
			continue
		}
		if err := r.insertSeriesEnrichmentRow(ctx, s.ID, en); err != nil {
			return err
		}
	}
	return nil
}
