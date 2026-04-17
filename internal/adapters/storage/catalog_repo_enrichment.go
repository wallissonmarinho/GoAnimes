package storage

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

// episodeMapsJSON mirrors episode_maps_json in DB (string keys for episode numbers).
type episodeMapsJSON struct {
	EpTitles   map[string]string `json:"ep_titles,omitempty"`
	EpThumbs   map[string]string `json:"ep_thumbs,omitempty"`
	EpReleased map[string]string `json:"ep_released_se,omitempty"`
	// Series-level Cinemeta fields (no extra DB columns; piggyback on episode_maps_json).
	SeriesStatus   string `json:"series_status,omitempty"`
	SeriesReleased string `json:"series_released,omitempty"`
	SeriesYear     string `json:"series_year,omitempty"`
}

func enrichmentEpisodeMapsJSON(en domain.SeriesEnrichment) (string, error) {
	p := episodeMapsJSON{EpTitles: map[string]string{}, EpThumbs: map[string]string{}}
	for k, v := range en.EpisodeTitleByNum {
		p.EpTitles[strconv.Itoa(k)] = v
	}
	for k, v := range en.EpisodeThumbnailByNum {
		p.EpThumbs[strconv.Itoa(k)] = v
	}
	if len(en.EpisodeReleasedBySeasonEpisode) > 0 {
		p.EpReleased = make(map[string]string)
		for k, v := range en.EpisodeReleasedBySeasonEpisode {
			if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
				continue
			}
			p.EpReleased[k] = v
		}
	}
	if s := strings.TrimSpace(en.SeriesStatus); s != "" {
		p.SeriesStatus = s
	}
	if s := strings.TrimSpace(en.SeriesReleasedISO); s != "" {
		p.SeriesReleased = s
	}
	if s := strings.TrimSpace(en.SeriesYearLabel); s != "" {
		p.SeriesYear = s
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "{}", err
	}
	return string(b), nil
}

func enrichmentFromEpisodeMapsJSON(raw string, en *domain.SeriesEnrichment) error {
	raw = trimJSON(raw)
	if raw == "" || raw == "{}" {
		en.EpisodeTitleByNum = map[int]string{}
		en.EpisodeThumbnailByNum = nil
		en.EpisodeReleasedBySeasonEpisode = nil
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
	if len(p.EpReleased) > 0 {
		en.EpisodeReleasedBySeasonEpisode = make(map[string]string)
		for k, v := range p.EpReleased {
			k = strings.TrimSpace(k)
			v = strings.TrimSpace(v)
			if k == "" || v == "" {
				continue
			}
			en.EpisodeReleasedBySeasonEpisode[k] = v
		}
	}
	en.SeriesStatus = strings.TrimSpace(p.SeriesStatus)
	en.SeriesReleasedISO = strings.TrimSpace(p.SeriesReleased)
	en.SeriesYearLabel = strings.TrimSpace(p.SeriesYear)
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

func (r *catalogRepo) insertSeriesEnrichmentRow(ctx context.Context, seriesID string, en domain.SeriesEnrichment) error {
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
				series_id, mal_id, imdb_id, tvdb_series_id,
				al_search_ver, next_air_unix, next_air_ep, next_air_from_al, start_year, episode_length_min,
				poster_url, background_url, al_banner_url, hero_bg_url, description, trailer_youtube_id,
				title_preferred, title_native, genres_json, episode_maps_json
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
			seriesID, en.MalID, en.ImdbID, en.TvdbSeriesID,
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
			series_id, mal_id, imdb_id, tvdb_series_id,
			al_search_ver, next_air_unix, next_air_ep, next_air_from_al, start_year, episode_length_min,
			poster_url, background_url, al_banner_url, hero_bg_url, description, trailer_youtube_id,
			title_preferred, title_native, genres_json, episode_maps_json
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		seriesID, en.MalID, en.ImdbID, en.TvdbSeriesID,
		en.AniListSearchVer, en.NextAiringUnix, en.NextAiringEpisode, nextFromAL, en.StartYear, en.EpisodeLengthMin,
		en.PosterURL, en.BackgroundURL, en.AniListBannerURL, en.StremioHeroBackgroundURL, en.Description, en.TrailerYouTubeID,
		en.TitlePreferred, en.TitleNative, string(gb), em)
	return err
}

func (r *catalogRepo) insertAllSeriesEnrichments(ctx context.Context, snap domain.CatalogSnapshot) error {
	if len(snap.SeriesEnrichmentBySeriesID) == 0 {
		return nil
	}
	seriesIDs := make(map[string]struct{}, len(snap.Series))
	for _, s := range snap.Series {
		seriesIDs[s.ID] = struct{}{}
	}
	for sid, en := range snap.SeriesEnrichmentBySeriesID {
		if _, ok := seriesIDs[sid]; !ok {
			continue
		}
		if err := r.insertSeriesEnrichmentRow(ctx, sid, en); err != nil {
			return err
		}
	}
	return nil
}

func (r *catalogRepo) loadAllSeriesEnrichments(ctx context.Context) (map[string]domain.SeriesEnrichment, error) {
	rows, err := r.ex.QueryContext(ctx, `
		SELECT series_id, mal_id, imdb_id, tvdb_series_id,
		       al_search_ver, next_air_unix, next_air_ep, next_air_from_al, start_year, episode_length_min,
		       poster_url, background_url, al_banner_url, hero_bg_url, description, trailer_youtube_id,
		       title_preferred, title_native, genres_json, episode_maps_json
		FROM series_enrichment`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]domain.SeriesEnrichment)
	for rows.Next() {
		var (
			sid, imdb, poster, bg, alBanner, hero, desc, trailer, titlePref, titleNat, gj, em string
			mal, tvdb, alVer, nextEp, startY, epLen                                           int
			nextUnix                                                                          int64
		)
		if r.pg {
			var nextFromAL bool
			if err := rows.Scan(&sid, &mal, &imdb, &tvdb,
				&alVer, &nextUnix, &nextEp, &nextFromAL, &startY, &epLen,
				&poster, &bg, &alBanner, &hero, &desc, &trailer, &titlePref, &titleNat, &gj, &em); err != nil {
				return nil, err
			}
			en := domain.SeriesEnrichment{
				PosterURL: poster, BackgroundURL: bg, AniListBannerURL: alBanner, StremioHeroBackgroundURL: hero,
				Description: desc, MalID: mal, ImdbID: imdb, TvdbSeriesID: tvdb,
				AniListSearchVer: alVer, NextAiringUnix: nextUnix, NextAiringEpisode: nextEp,
				NextAiringFromAniList: nextFromAL, StartYear: startY, EpisodeLengthMin: epLen, TrailerYouTubeID: trailer,
				TitlePreferred: titlePref, TitleNative: titleNat,
			}
			_ = json.Unmarshal([]byte(gj), &en.Genres)
			if err := enrichmentFromEpisodeMapsJSON(em, &en); err != nil {
				en.EpisodeTitleByNum = map[int]string{}
			}
			out[sid] = en
		} else {
			var nextALInt int
			if err := rows.Scan(&sid, &mal, &imdb, &tvdb,
				&alVer, &nextUnix, &nextEp, &nextALInt, &startY, &epLen,
				&poster, &bg, &alBanner, &hero, &desc, &trailer, &titlePref, &titleNat, &gj, &em); err != nil {
				return nil, err
			}
			en := domain.SeriesEnrichment{
				PosterURL: poster, BackgroundURL: bg, AniListBannerURL: alBanner, StremioHeroBackgroundURL: hero,
				Description: desc, MalID: mal, ImdbID: imdb, TvdbSeriesID: tvdb,
				AniListSearchVer: alVer, NextAiringUnix: nextUnix, NextAiringEpisode: nextEp,
				NextAiringFromAniList: nextALInt != 0, StartYear: startY, EpisodeLengthMin: epLen, TrailerYouTubeID: trailer,
				TitlePreferred: titlePref, TitleNative: titleNat,
			}
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
	if len(snap.SeriesEnrichmentBySeriesID) == 0 {
		return nil
	}
	for _, s := range snap.Series {
		en, ok := snap.SeriesEnrichmentBySeriesID[s.ID]
		if !ok {
			continue
		}
		if err := r.insertSeriesEnrichmentRow(ctx, s.ID, en); err != nil {
			return err
		}
	}
	return nil
}
