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
	// Resolve Kitsu id per series, then one FetchEpisodeMaps per distinct kid (many series share the same Kitsu row).
	kidToSeriesIDs := make(map[string][]string)
	for _, ser := range series {
		select {
		case <-ctx.Done():
			s.log.Warn("kitsu episodes: stopped early (context done)", slog.Int("phase", 0))
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
		kidToSeriesIDs[kid] = append(kidToSeriesIDs[kid], ser.ID)
	}
	kids := make([]string, 0, len(kidToSeriesIDs))
	for k := range kidToSeriesIDs {
		kids = append(kids, k)
	}
	sort.Strings(kids)
	fetches, seriesUpdated := 0, 0
	for _, kid := range kids {
		select {
		case <-ctx.Done():
			s.log.Warn("kitsu episodes: stopped early (context done)", slog.Int("fetches", fetches))
			appendSyncNote(syncNotes, "kitsu episodes: stopped early (context cancelled)")
			return
		default:
		}
		titles, thumbs, err := s.kitsu.FetchEpisodeMaps(ctx, kid)
		if err != nil {
			s.log.Debug("kitsu episodes failed", slog.String("kitsu_id", kid), slog.Any("err", err))
			appendSyncNote(syncNotes, fmt.Sprintf("kitsu episodes %s: %v", kid, err))
			continue
		}
		if len(titles) == 0 && len(thumbs) == 0 {
			continue
		}
		fetches++
		ids := append([]string(nil), kidToSeriesIDs[kid]...)
		sort.Strings(ids)
		add := domain.AniListSeriesEnrichment{
			KitsuAnimeID:          kid,
			EpisodeTitleByNum:     titles,
			EpisodeThumbnailByNum: thumbs,
		}
		for _, sid := range ids {
			cache[sid] = domain.MergeAniListEnrichment(cache[sid], add)
			seriesUpdated++
		}
		if s.kitsuDelay > 0 {
			time.Sleep(s.kitsuDelay)
		}
	}
	if fetches > 0 {
		s.log.Info("kitsu: merged episode titles/thumbnails",
			slog.Int("kitsu_fetches", fetches), slog.Int("series_rows", seriesUpdated))
	}
}

func (s *RSSSyncService) enrichJikanMalEpisodeTitles(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 || s.jikan == nil {
		return
	}
	// One Jikan episode-list fetch per distinct MalID (after Mal-merge many series rows share the same MAL id).
	malToSeriesIDs := make(map[int][]string)
	for _, ser := range series {
		cur := cache[ser.ID]
		if cur.MalID <= 0 {
			continue
		}
		malToSeriesIDs[cur.MalID] = append(malToSeriesIDs[cur.MalID], ser.ID)
	}
	malIDs := make([]int, 0, len(malToSeriesIDs))
	for mal := range malToSeriesIDs {
		malIDs = append(malIDs, mal)
	}
	sort.Ints(malIDs)
	fetches, seriesUpdated := 0, 0
	for _, malID := range malIDs {
		select {
		case <-ctx.Done():
			s.log.Warn("jikan mal episodes: stopped early (context done)", slog.Int("fetches", fetches))
			appendSyncNote(syncNotes, "jikan mal episodes: stopped early (context cancelled)")
			return
		default:
		}
		eps, err := s.jikan.FetchEpisodeTitlesByMalID(ctx, malID)
		if err != nil {
			s.log.Debug("jikan mal episodes failed", slog.Int("mal_id", malID), slog.Any("err", err))
			appendSyncNote(syncNotes, fmt.Sprintf("jikan mal episodes %d: %v", malID, err))
			continue
		}
		if len(eps) == 0 {
			continue
		}
		fetches++
		add := domain.AniListSeriesEnrichment{EpisodeTitleByNum: eps}
		ids := append([]string(nil), malToSeriesIDs[malID]...)
		sort.Strings(ids)
		for _, sid := range ids {
			cache[sid] = domain.MergeAniListEnrichment(cache[sid], add)
			seriesUpdated++
		}
		if s.jikanDelay > 0 {
			time.Sleep(s.jikanDelay)
		}
	}
	if fetches > 0 {
		s.log.Info("jikan: merged MAL episode titles by mal_id",
			slog.Int("mal_fetches", fetches), slog.Int("series_rows", seriesUpdated))
	}
}

func (s *RSSSyncService) enrichAniDBEpisodeTitles(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 || s.anidb == nil {
		return
	}
	now := time.Now().Unix()
	const ttlSec int64 = 86400
	aidToSeriesIDs := make(map[int][]string)
	for _, ser := range series {
		cur := cache[ser.ID]
		if cur.AniDBAid <= 0 {
			continue
		}
		aidToSeriesIDs[cur.AniDBAid] = append(aidToSeriesIDs[cur.AniDBAid], ser.ID)
	}
	aids := make([]int, 0, len(aidToSeriesIDs))
	for aid := range aidToSeriesIDs {
		aids = append(aids, aid)
	}
	sort.Ints(aids)
	fetches, seriesUpdated := 0, 0
	for _, aid := range aids {
		select {
		case <-ctx.Done():
			s.log.Warn("anidb episodes: stopped early (context done)", slog.Int("fetches", fetches))
			appendSyncNote(syncNotes, "anidb episodes: stopped early (context cancelled)")
			return
		default:
		}
		ids := aidToSeriesIDs[aid]
		needFetch := false
		for _, sid := range ids {
			cur := cache[sid]
			if cur.AniDBLastFetchedUnix == 0 || now-cur.AniDBLastFetchedUnix >= ttlSec {
				needFetch = true
				break
			}
		}
		if !needFetch {
			continue
		}
		titles, err := s.anidb.FetchEpisodeTitlesByAID(ctx, aid)
		if err != nil {
			s.log.Debug("anidb episodes failed", slog.Int("aid", aid), slog.Any("err", err))
			appendSyncNote(syncNotes, fmt.Sprintf("anidb aid %d: %v", aid, err))
			continue
		}
		fetches++
		add := domain.AniListSeriesEnrichment{AniDBLastFetchedUnix: now}
		if len(titles) > 0 {
			add.EpisodeTitleByNum = titles
		}
		sort.Strings(ids)
		for _, sid := range ids {
			cache[sid] = domain.MergeAniListEnrichment(cache[sid], add)
			seriesUpdated++
		}
		if s.anidbDelay > 0 {
			time.Sleep(s.anidbDelay)
		}
	}
	if fetches > 0 {
		s.log.Info("anidb: merged episode titles", slog.Int("anidb_fetches", fetches), slog.Int("series_rows", seriesUpdated))
	}
}

func (s *RSSSyncService) enrichTheTVDBGaps(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.AniListSeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 || s.tvdb == nil {
		return
	}
	imdbToSeriesIDs := make(map[string][]string)
	for _, ser := range series {
		cur := cache[ser.ID]
		imdb := domain.NormalizeIMDbID(cur.ImdbID)
		if imdb == "" {
			continue
		}
		imdbToSeriesIDs[imdb] = append(imdbToSeriesIDs[imdb], ser.ID)
	}
	keys := make([]string, 0, len(imdbToSeriesIDs))
	for imdb := range imdbToSeriesIDs {
		keys = append(keys, imdb)
	}
	sort.Strings(keys)
	fetches, seriesUpdated := 0, 0
	for i, imdb := range keys {
		select {
		case <-ctx.Done():
			s.log.Warn("thetvdb: stopped early (context done)", slog.Int("fetches", fetches))
			appendSyncNote(syncNotes, "thetvdb: stopped early (context cancelled)")
			return
		default:
		}
		if i > 0 && s.tvdbDelay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.tvdbDelay):
			}
		}
		ids := imdbToSeriesIDs[imdb]
		sort.Strings(ids)
		repID := ids[0]
		rep := cache[repID]
		tvdbID := rep.TvdbSeriesID
		if tvdbID <= 0 {
			var err error
			tvdbID, err = s.tvdb.SeriesIDByIMDbRemote(ctx, imdb)
			if err != nil {
				s.log.Debug("thetvdb remote id failed", slog.String("imdb", imdb), slog.Any("err", err))
				appendSyncNote(syncNotes, fmt.Sprintf("thetvdb imdb %s: %v", imdb, err))
				continue
			}
			fetches++
		}
		if tvdbID <= 0 {
			continue
		}
		skipEpisodes := rep.TvdbSeriesID == tvdbID && tvdbID > 0 &&
			domain.EnrichmentHasAnyEpisodeTitle(rep) && len(rep.EpisodeThumbnailByNum) > 0
		var titles, thumbs map[int]string
		if !skipEpisodes {
			if s.tvdbDelay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(s.tvdbDelay):
				}
			}
			var err error
			titles, thumbs, err = s.tvdb.EpisodeMapsOfficial(ctx, tvdbID)
			if err != nil {
				s.log.Debug("thetvdb episodes failed", slog.Int("tvdb_series_id", tvdbID), slog.Any("err", err))
				appendSyncNote(syncNotes, fmt.Sprintf("thetvdb series %d episodes: %v", tvdbID, err))
			} else {
				fetches++
			}
		}
		add := domain.AniListSeriesEnrichment{TvdbSeriesID: tvdbID}
		if len(titles) > 0 {
			add.EpisodeTitleByNum = titles
		}
		if len(thumbs) > 0 {
			add.EpisodeThumbnailByNum = thumbs
		}
		for _, sid := range ids {
			cache[sid] = domain.MergeAniListEnrichment(cache[sid], add)
			seriesUpdated++
		}
	}
	if fetches > 0 {
		s.log.Info("thetvdb: merged series id / episode maps", slog.Int("http_roundtrips", fetches), slog.Int("series_rows", seriesUpdated))
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
		s.log.Info("tmdb: skipped (no API key or GOANIMES_TMDB_DISABLED; hero uses AniList/Kitsu/TheTVDB only)")
	}
	if s.tvdb == nil {
		s.log.Info("thetvdb: skipped (no GOANIMES_TVDB_API_KEY or GOANIMES_TVDB_DISABLED)")
	}
	if s.tmdb == nil && s.tvdb == nil {
		return
	}
	// TMDB/TheTVDB HTTP is expensive; reuse backdrop candidates for all series that share the same lookup key (IMDb, or MAL id).
	type tmdbFetchRep struct {
		en     domain.AniListSeriesEnrichment
		search string
		name   string
	}
	tmdbKeyOrder := make([]string, 0)
	tmdbKeyRep := make(map[string]tmdbFetchRep)
	tmdbCandsByKey := make(map[string][]domain.BackgroundCandidate)
	tvdbCandsByKey := make(map[string][]domain.BackgroundCandidate)
	for _, ser := range series {
		en := cache[ser.ID]
		key := heroTMDBFetchKey(en, ser.ID)
		if _, ok := tmdbKeyRep[key]; ok {
			continue
		}
		tmdbKeyRep[key] = tmdbFetchRep{en: en, search: ser.Name, name: ser.Name}
		tmdbKeyOrder = append(tmdbKeyOrder, key)
	}
	sort.Strings(tmdbKeyOrder)
	tmdbFetches, tvdbFetches := 0, 0
	for i, key := range tmdbKeyOrder {
		select {
		case <-ctx.Done():
			s.log.Warn("stremio hero: stopped early (context done)", slog.Int("tmdb_fetches", tmdbFetches))
			appendSyncNote(syncNotes, "stremio hero: stopped early (context cancelled)")
			return
		default:
		}
		rep := tmdbKeyRep[key]
		var cands []domain.BackgroundCandidate
		if s.tmdb != nil {
			if i > 0 && s.tmdbDelay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(s.tmdbDelay):
				}
			}
			var err error
			cands, err = services.TMDBBackdropCandidatesForEnrichment(ctx, s.tmdb, rep.en, rep.search)
			if err != nil {
				s.log.Debug("tmdb backdrop fetch failed", slog.String("series", rep.name), slog.Any("err", err))
				appendSyncNote(syncNotes, fmt.Sprintf("tmdb %q: %v", rep.name, err))
			} else {
				tmdbFetches++
			}
		}
		tmdbCandsByKey[key] = cands

		var tvCands []domain.BackgroundCandidate
		if s.tvdb != nil {
			if s.tvdbDelay > 0 && (i > 0 || s.tmdb != nil) {
				select {
				case <-ctx.Done():
					return
				case <-time.After(s.tvdbDelay):
				}
			}
			tv, err := services.TVDBBackdropCandidatesForEnrichment(ctx, s.tvdb, rep.en)
			if err != nil {
				s.log.Debug("thetvdb backdrop fetch failed", slog.String("series", rep.name), slog.Any("err", err))
				appendSyncNote(syncNotes, fmt.Sprintf("thetvdb %q: %v", rep.name, err))
			} else {
				tvdbFetches++
				tvCands = tv
			}
		}
		tvdbCandsByKey[key] = tvCands
	}
	n := 0
	for _, ser := range series {
		select {
		case <-ctx.Done():
			s.log.Warn("stremio hero: stopped early (context done)", slog.Int("resolved", n))
			appendSyncNote(syncNotes, "stremio hero: stopped early (context cancelled)")
			return
		default:
		}
		en := cache[ser.ID]
		key := heroTMDBFetchKey(en, ser.ID)
		tmdbCands := tmdbCandsByKey[key]
		tvCands := tvdbCandsByKey[key]
		combined := append(append([]domain.BackgroundCandidate{}, tmdbCands...), tvCands...)
		hero := strings.TrimSpace(domain.ResolveStremioHeroBackground(en, combined))
		if hero == "" {
			continue
		}
		en.StremioHeroBackgroundURL = hero
		cache[ser.ID] = en
		n++
	}
	if n > 0 {
		s.log.Info("stremio hero: resolved backgrounds",
			slog.Int("series", n), slog.Int("tmdb_fetches", tmdbFetches), slog.Int("thetvdb_fetches", tvdbFetches),
			slog.Bool("tmdb", s.tmdb != nil), slog.Bool("thetvdb", s.tvdb != nil))
	}
}

// heroTMDBFetchKey groups series for a single TMDB request (IMDb id preferred, else MAL id, else unique per Stremio series id).
func heroTMDBFetchKey(en domain.AniListSeriesEnrichment, seriesID string) string {
	if imdb := domain.NormalizeIMDbID(en.ImdbID); imdb != "" {
		return "imdb:" + imdb
	}
	if en.MalID > 0 {
		return fmt.Sprintf("mal:%d", en.MalID)
	}
	return "series:" + seriesID
}
