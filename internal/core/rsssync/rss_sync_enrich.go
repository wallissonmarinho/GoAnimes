package rsssync

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/anilist"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/services"
)

func (s *RSSSyncService) enrichAniListSeries(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 {
		return
	}
	if s.anilist == nil {
		s.log.Info("anilist: skipped (set GOANIMES_ANILIST_DISABLED=true to disable; no client)")
		return
	}
	missing := 0
	for _, ser := range series {
		if domain.AniListNeedsRefetch(cache[ser.ID]) {
			missing++
		}
	}
	if missing == 0 {
		s.log.Info("anilist: all series have full cached metadata", slog.Int("series", len(series)))
		return
	}
	s.log.Info("anilist: fetching metadata (posters, synopsis, genres, …)",
		slog.Int("to_fetch", missing),
		slog.Int("series_total", len(series)),
		slog.Duration("min_delay_between_requests", s.anilistDelay))

	newN, fails := 0, 0
	for _, ser := range series {
		select {
		case <-ctx.Done():
			s.log.Warn("anilist: stopped early (context done)", slog.Int("new_rows", newN), slog.Int("failures", fails))
			appendSyncNote(syncNotes, "anilist: stopped early (context cancelled)")
			return
		default:
		}
		if !domain.AniListNeedsRefetch(cache[ser.ID]) {
			continue
		}
		det, err := s.anilist.SearchAnimeMedia(ctx, ser.Name)
		if err != nil {
			fails++
			qlog := domain.NormalizeExternalAnimeSearchQuery(ser.Name)
			s.log.Warn("anilist lookup failed", slog.String("series", ser.Name), slog.String("search_query", qlog), slog.Any("err", err))
			cur := cache[ser.ID]
			if s.jikan != nil && domain.EnrichmentCouldUseJikan(cur) {
				add, jerr := s.jikan.SearchAnimeEnrichment(ctx, ser.Name)
				if jerr == nil {
					merged := domain.MergeAniListEnrichment(cur, add)
					merged.Description = services.TranslateSynopsisToPT(s.synopsisTrans, s.log, merged.Description)
					cache[ser.ID] = merged
					newN++
					s.log.Info("jikan: filled series after anilist miss", slog.String("series", ser.Name), slog.String("search_query", qlog))
					if s.jikanDelay > 0 {
						time.Sleep(s.jikanDelay)
					}
					continue
				}
				s.log.Debug("jikan fallback after anilist failure", slog.String("series", ser.Name), slog.Any("err", jerr))
				if s.kitsu != nil && domain.EnrichmentCouldUseJikan(cur) {
					addK, kerr := s.kitsu.SearchAnimeEnrichment(ctx, ser.Name)
					if kerr == nil {
						merged := domain.MergeAniListEnrichment(cur, addK)
						merged.Description = services.TranslateSynopsisToPT(s.synopsisTrans, s.log, merged.Description)
						cache[ser.ID] = merged
						newN++
						s.log.Info("kitsu: filled series after anilist+jikan miss", slog.String("series", ser.Name), slog.String("search_query", qlog))
						if s.kitsuDelay > 0 {
							time.Sleep(s.kitsuDelay)
						}
						continue
					}
					appendSyncNote(syncNotes, fmt.Sprintf("enrichment %q: anilist: %v; jikan: %v; kitsu: %v", qlog, err, jerr, kerr))
					continue
				}
				appendSyncNote(syncNotes, fmt.Sprintf("enrichment %q: anilist: %v; jikan: %v", qlog, err, jerr))
				continue
			}
			if s.kitsu != nil && domain.EnrichmentCouldUseJikan(cur) {
				addK, kerr := s.kitsu.SearchAnimeEnrichment(ctx, ser.Name)
				if kerr == nil {
					merged := domain.MergeAniListEnrichment(cur, addK)
					merged.Description = services.TranslateSynopsisToPT(s.synopsisTrans, s.log, merged.Description)
					cache[ser.ID] = merged
					newN++
					s.log.Info("kitsu: filled series after anilist miss (no jikan)", slog.String("series", ser.Name), slog.String("search_query", qlog))
					if s.kitsuDelay > 0 {
						time.Sleep(s.kitsuDelay)
					}
					continue
				}
				appendSyncNote(syncNotes, fmt.Sprintf("enrichment %q: anilist: %v; kitsu: %v", qlog, err, kerr))
				continue
			}
			appendSyncNote(syncNotes, fmt.Sprintf("anilist %q: %v", qlog, err))
			continue
		}
		en := anilist.ToDomainEnrichment(det)
		en.Description = services.TranslateSynopsisToPT(s.synopsisTrans, s.log, en.Description)
		cache[ser.ID] = en
		newN++
		if s.anilistDelay > 0 {
			time.Sleep(s.anilistDelay)
		}
	}
	s.log.Info("anilist: finished", slog.Int("new_or_refreshed", newN), slog.Int("lookup_failures", fails))
}

func (s *RSSSyncService) enrichJikanGaps(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 || s.jikan == nil {
		if s.jikan == nil {
			s.log.Info("jikan: skipped (set GOANIMES_JIKAN_DISABLED=true to disable; no client)")
		}
		return
	}
	n := 0
	for _, ser := range series {
		select {
		case <-ctx.Done():
			s.log.Warn("jikan: stopped early (context done)", slog.Int("merged", n))
			appendSyncNote(syncNotes, "jikan: stopped early (context cancelled)")
			return
		default:
		}
		if strings.TrimSpace(ser.Name) == "" {
			continue
		}
		cur := cache[ser.ID]
		if !domain.EnrichmentCouldUseJikan(cur) {
			continue
		}
		add, err := s.jikan.SearchAnimeEnrichment(ctx, ser.Name)
		if err != nil {
			s.log.Debug("jikan lookup skipped or failed", slog.String("series", ser.Name), slog.Any("err", err))
			appendSyncNote(syncNotes, fmt.Sprintf("jikan %q: %v", ser.Name, err))
			continue
		}
		merged := domain.MergeAniListEnrichment(cur, add)
		merged.Description = services.TranslateSynopsisToPT(s.synopsisTrans, s.log, merged.Description)
		cache[ser.ID] = merged
		n++
		if s.jikanDelay > 0 {
			time.Sleep(s.jikanDelay)
		}
	}
	if n > 0 {
		s.log.Info("jikan: filled gaps", slog.Int("series", n))
	}
}

func (s *RSSSyncService) enrichKitsuGaps(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 || s.kitsu == nil {
		if s.kitsu == nil {
			s.log.Info("kitsu: skipped (set GOANIMES_KITSU_DISABLED=true to disable; no client)")
		}
		return
	}
	n := 0
	for _, ser := range series {
		select {
		case <-ctx.Done():
			s.log.Warn("kitsu: stopped early (context done)", slog.Int("merged", n))
			appendSyncNote(syncNotes, "kitsu: stopped early (context cancelled)")
			return
		default:
		}
		if strings.TrimSpace(ser.Name) == "" {
			continue
		}
		cur := cache[ser.ID]
		if !domain.EnrichmentCouldUseJikan(cur) {
			continue
		}
		add, err := s.kitsu.SearchAnimeEnrichment(ctx, ser.Name)
		if err != nil {
			s.log.Debug("kitsu lookup skipped or failed", slog.String("series", ser.Name), slog.Any("err", err))
			appendSyncNote(syncNotes, fmt.Sprintf("kitsu %q: %v", ser.Name, err))
			continue
		}
		merged := domain.MergeAniListEnrichment(cur, add)
		merged.Description = services.TranslateSynopsisToPT(s.synopsisTrans, s.log, merged.Description)
		cache[ser.ID] = merged
		n++
		if s.kitsuDelay > 0 {
			time.Sleep(s.kitsuDelay)
		}
	}
	if n > 0 {
		s.log.Info("kitsu: filled gaps", slog.Int("series", n))
	}
}

func (s *RSSSyncService) enrichKitsuEpisodeMaps(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 || s.kitsu == nil {
		return
	}
	n := 0
	for _, ser := range series {
		select {
		case <-ctx.Done():
			s.log.Warn("kitsu episodes: stopped early (context done)", slog.Int("merged", n))
			appendSyncNote(syncNotes, "kitsu episodes: stopped early (context cancelled)")
			return
		default:
		}
		cur := cache[ser.ID]
		kid := strings.TrimSpace(cur.KitsuAnimeID)
		if kid == "" && strings.TrimSpace(ser.Name) != "" {
			id, err := s.kitsu.SearchAnimeID(ctx, ser.Name)
			if err == nil && id != "" {
				cur.KitsuAnimeID = id
				cache[ser.ID] = cur
				kid = id
			}
		}
		if kid == "" {
			continue
		}
		titles, thumbs, err := s.kitsu.FetchEpisodeMaps(ctx, kid)
		if err != nil {
			s.log.Debug("kitsu episodes failed", slog.String("kitsu_id", kid), slog.String("series", ser.Name), slog.Any("err", err))
			appendSyncNote(syncNotes, fmt.Sprintf("kitsu episodes %s: %v", kid, err))
			continue
		}
		if len(titles) == 0 && len(thumbs) == 0 {
			continue
		}
		cache[ser.ID] = domain.MergeAniListEnrichment(cache[ser.ID], domain.AniListSeriesEnrichment{
			KitsuAnimeID:          kid,
			EpisodeTitleByNum:     titles,
			EpisodeThumbnailByNum: thumbs,
		})
		n++
		if s.kitsuDelay > 0 {
			time.Sleep(s.kitsuDelay)
		}
	}
	if n > 0 {
		s.log.Info("kitsu: merged episode titles/thumbnails", slog.Int("series", n))
	}
}

func (s *RSSSyncService) enrichJikanMalEpisodeTitles(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 || s.jikan == nil {
		return
	}
	n := 0
	for _, ser := range series {
		select {
		case <-ctx.Done():
			s.log.Warn("jikan mal episodes: stopped early (context done)", slog.Int("merged", n))
			appendSyncNote(syncNotes, "jikan mal episodes: stopped early (context cancelled)")
			return
		default:
		}
		cur := cache[ser.ID]
		if cur.MalID <= 0 {
			continue
		}
		eps, err := s.jikan.FetchEpisodeTitlesByMalID(ctx, cur.MalID)
		if err != nil {
			s.log.Debug("jikan mal episodes failed", slog.Int("mal_id", cur.MalID), slog.String("series", ser.Name), slog.Any("err", err))
			appendSyncNote(syncNotes, fmt.Sprintf("jikan mal episodes %d: %v", cur.MalID, err))
			continue
		}
		if len(eps) == 0 {
			continue
		}
		cache[ser.ID] = domain.MergeAniListEnrichment(cur, domain.AniListSeriesEnrichment{EpisodeTitleByNum: eps})
		n++
		if s.jikanDelay > 0 {
			time.Sleep(s.jikanDelay)
		}
	}
	if n > 0 {
		s.log.Info("jikan: merged MAL episode titles by mal_id", slog.Int("series", n))
	}
}

func (s *RSSSyncService) enrichAniDBEpisodeTitles(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 || s.anidb == nil {
		return
	}
	now := time.Now().Unix()
	const ttlSec int64 = 86400
	n := 0
	for _, ser := range series {
		select {
		case <-ctx.Done():
			s.log.Warn("anidb episodes: stopped early (context done)", slog.Int("merged", n))
			appendSyncNote(syncNotes, "anidb episodes: stopped early (context cancelled)")
			return
		default:
		}
		cur := cache[ser.ID]
		if cur.AniDBAid <= 0 {
			continue
		}
		if cur.AniDBLastFetchedUnix > 0 && now-cur.AniDBLastFetchedUnix < ttlSec {
			continue
		}
		titles, err := s.anidb.FetchEpisodeTitlesByAID(ctx, cur.AniDBAid)
		if err != nil {
			s.log.Debug("anidb episodes failed", slog.Int("aid", cur.AniDBAid), slog.String("series", ser.Name), slog.Any("err", err))
			appendSyncNote(syncNotes, fmt.Sprintf("anidb aid %d: %v", cur.AniDBAid, err))
			continue
		}
		add := domain.AniListSeriesEnrichment{AniDBLastFetchedUnix: now}
		if len(titles) > 0 {
			add.EpisodeTitleByNum = titles
		}
		cache[ser.ID] = domain.MergeAniListEnrichment(cur, add)
		n++
		if s.anidbDelay > 0 {
			time.Sleep(s.anidbDelay)
		}
	}
	if n > 0 {
		s.log.Info("anidb: merged episode titles", slog.Int("series", n))
	}
}

func (s *RSSSyncService) translateEpisodeTitlesToPT(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 || s.synopsisTrans == nil {
		return
	}
	const pace = 75 * time.Millisecond
	seriesChanged, titlesTranslated := 0, 0
	for _, ser := range series {
		select {
		case <-ctx.Done():
			s.log.Warn("episode title translate: stopped early (context done)", slog.Int("titles", titlesTranslated))
			appendSyncNote(syncNotes, "episode title translate: stopped early (context cancelled)")
			return
		default:
		}
		cur := cache[ser.ID]
		if len(cur.EpisodeTitleByNum) == 0 {
			continue
		}
		next := maps.Clone(cur.EpisodeTitleByNum)
		keys := make([]int, 0, len(next))
		for k := range next {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		changed := false
		for _, k := range keys {
			select {
			case <-ctx.Done():
				s.log.Warn("episode title translate: stopped early (context done)", slog.Int("titles", titlesTranslated))
				appendSyncNote(syncNotes, "episode title translate: stopped early (context cancelled)")
				return
			default:
			}
			before := next[k]
			if !domain.EpisodeTitleWorthTranslating(before) {
				continue
			}
			after := services.TranslateEpisodeTitleToPT(s.synopsisTrans, s.log, before)
			next[k] = after
			if after != before {
				changed = true
				titlesTranslated++
			}
			if pace > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(pace):
				}
			}
		}
		if changed {
			cur.EpisodeTitleByNum = next
			cache[ser.ID] = cur
			seriesChanged++
		}
	}
	if titlesTranslated > 0 {
		s.log.Info("episode titles translated to pt-BR", slog.Int("titles", titlesTranslated), slog.Int("series", seriesChanged))
	}
}

func (s *RSSSyncService) resolveStremioHeroBackgrounds(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 {
		return
	}
	if s.tmdb == nil {
		s.log.Info("tmdb: skipped (no API key or GOANIMES_TMDB_DISABLED; hero uses AniList/Kitsu only)")
	}
	n := 0
	for i, ser := range series {
		select {
		case <-ctx.Done():
			s.log.Warn("stremio hero: stopped early (context done)", slog.Int("resolved", n))
			appendSyncNote(syncNotes, "stremio hero: stopped early (context cancelled)")
			return
		default:
		}
		en := cache[ser.ID]
		search := ser.Name
		var tmdbCands []domain.BackgroundCandidate
		if s.tmdb != nil {
			cands, err := services.TMDBBackdropCandidatesForEnrichment(ctx, s.tmdb, en, search)
			if err != nil {
				s.log.Debug("tmdb backdrop fetch failed", slog.String("series", ser.Name), slog.Any("err", err))
				appendSyncNote(syncNotes, fmt.Sprintf("tmdb %q: %v", ser.Name, err))
			} else {
				tmdbCands = cands
			}
			if i > 0 && s.tmdbDelay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(s.tmdbDelay):
				}
			}
		}
		hero := strings.TrimSpace(domain.ResolveStremioHeroBackground(en, tmdbCands))
		if hero == "" {
			continue
		}
		en.StremioHeroBackgroundURL = hero
		cache[ser.ID] = en
		n++
	}
	if n > 0 {
		s.log.Info("stremio hero: resolved backgrounds", slog.Int("series", n), slog.Bool("tmdb", s.tmdb != nil))
	}
}
