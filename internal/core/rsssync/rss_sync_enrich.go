package rsssync

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/cinemeta"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/services"
)

var (
	cinemetaBracket    = regexp.MustCompile(`\[[^\]]*\]|\([^\)]*\)`)
	cinemetaNoise      = regexp.MustCompile(`(?i)\b(1080p|720p|2160p|540p|480p|sd|hevc|x265|x264|avc|aac|eac3|webrip|web[- ]?dl|bluray|multisub|multi(sub|audio)?|cr|torrent|mkv|mp4|amzn|dsnp|nf|adn|hidive|batch|repack|encoded|airing|chinese\s+audio|ca)\b`)
	cinemetaSxE        = regexp.MustCompile(`(?i)\bS(\d{1,2})\s*[-Ex ]\s*(\d{1,3})\b`)
	cinemetaDashEp     = regexp.MustCompile(`\s-\s(\d{1,3})(?:\D|$)`)
	cinemetaVerTag     = regexp.MustCompile(`(?i)\bv\d+\b`)
	cinemetaRangeEp    = regexp.MustCompile(`(?i)\b\d{1,4}\s*~\s*\d{1,4}\b`)
	cinemetaRangeTail  = regexp.MustCompile(`(?i)\s*~\s*\d{1,4}(?:v\d+)?\b`)
	cinemetaSeasonTail = regexp.MustCompile(`(?i)\s+(?:(?:\d{1,2}(?:st|nd|rd|th)\s+season)|(?:season\s+\d{1,2})|(?:s\d{1,2})|(?:part\s+\d{1,2})|(?:cour\s+\d{1,2})|(?:act\s+[ivx]+)|(?:final\s+season))\s*$`)
	cinemetaSpace      = regexp.MustCompile(`\s+`)
)

func (s *RSSSyncService) enrichCinemetaGaps(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.SeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 || s.cinemeta == nil {
		return
	}
	updated := 0
	for i, ser := range series {
		select {
		case <-ctx.Done():
			s.log.Warn("cinemeta: stopped early (context done)", slog.Int("updated", updated))
			appendSyncNote(syncNotes, "cinemeta: stopped early (context cancelled)")
			return
		default:
		}
		if i > 0 && s.cinemetaDelay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.cinemetaDelay):
			}
		}
		cur := cache[ser.ID]
		queries := cinemetaQueriesForSeries(ser.Name)
		if len(queries) == 0 {
			continue
		}
		meta, err := s.cinemetaResolveSeries(ctx, queries)
		if err != nil || meta == nil {
			if err != nil {
				appendSyncNote(syncNotes, fmt.Sprintf("cinemeta %q: %v", ser.Name, err))
			}
			continue
		}
		add := domain.SeriesEnrichment{
			AniListSearchVer:               domain.ExternalMetadataSearchVersion,
			PosterURL:                      strings.TrimSpace(meta.Poster),
			BackgroundURL:                  strings.TrimSpace(meta.Background),
			Description:                    strings.TrimSpace(meta.Description),
			ImdbID:                         domain.NormalizeIMDbID(firstNonEmpty(meta.IMDBID, meta.ID)),
			TvdbSeriesID:                   meta.TVDBID,
			TitlePreferred:                 strings.TrimSpace(meta.Name),
			SeriesStatus:                   strings.TrimSpace(meta.Status),
			SeriesReleasedISO:              strings.TrimSpace(meta.Released),
			SeriesYearLabel:                strings.TrimSpace(meta.Year),
			EpisodeTitleByNum:              map[int]string{},
			EpisodeThumbnailByNum:          map[int]string{},
			EpisodeReleasedBySeasonEpisode: map[string]string{},
		}
		if add.SeriesYearLabel == "" {
			add.SeriesYearLabel = strings.TrimSpace(meta.ReleaseInfo)
		}
		if len(meta.Genre) > 0 {
			add.Genres = append([]string(nil), meta.Genre...)
		}
		add.StartYear = extractStartYear(firstNonEmpty(meta.ReleaseInfo, meta.Year))
		for _, v := range meta.Videos {
			epNum := v.Episode
			if epNum <= 0 {
				epNum = v.Number
			}
			if epNum <= 0 {
				continue
			}
			add.EpisodeTitleByNum[epNum] = strings.TrimSpace(v.Name)
			add.EpisodeThumbnailByNum[epNum] = strings.TrimSpace(v.Thumbnail)
			numInSeason := v.Number
			if numInSeason <= 0 {
				numInSeason = v.Episode
			}
			if numInSeason > 0 {
				air := strings.TrimSpace(v.Released)
				if air == "" {
					air = strings.TrimSpace(v.FirstAired)
				}
				if air != "" {
					sk := domain.SeasonEpisodeScheduleKey(v.Season, numInSeason)
					add.EpisodeReleasedBySeasonEpisode[sk] = air
				}
			}
		}
		cache[ser.ID] = domain.MergeSeriesEnrichment(cur, add)
		updated++
	}
	if updated > 0 {
		s.log.Info("cinemeta: merged metadata", slog.Int("series_rows", updated))
	}
}

func (s *RSSSyncService) cinemetaResolveSeries(ctx context.Context, queries []string) (*cinemeta.SeriesMeta, error) {
	var bestID string
	bestScore := -1.0
	for _, q := range queries {
		hits, err := s.cinemeta.SearchSeries(ctx, q)
		if err != nil {
			continue
		}
		for _, h := range hits {
			sc := scoreCinemetaCandidate(q, h.Name)
			if sc > bestScore {
				bestScore = sc
				bestID = strings.TrimSpace(h.ID)
			}
		}
	}
	if bestID == "" {
		return nil, nil
	}
	meta, err := s.cinemeta.GetSeriesMeta(ctx, bestID)
	if err != nil || meta == nil {
		return nil, err
	}
	return meta, nil
}

func cinemetaQueriesForSeries(name string) []string {
	raw := strings.TrimSpace(name)
	if raw == "" {
		return nil
	}
	full := cinemetaBracket.ReplaceAllString(raw, " ")
	full = cinemetaNoise.ReplaceAllString(full, " ")
	full = cinemetaSxE.ReplaceAllString(full, " ")
	full = cinemetaDashEp.ReplaceAllString(full, " ")
	full = cinemetaVerTag.ReplaceAllString(full, " ")
	full = cinemetaRangeEp.ReplaceAllString(full, " ")
	full = cinemetaRangeTail.ReplaceAllString(full, " ")
	full = strings.ReplaceAll(full, "_", " ")
	full = cinemetaSpace.ReplaceAllString(full, " ")
	full = strings.TrimSpace(full)
	if full == "" {
		return nil
	}
	// Prefer short canonical title first (better retrieval quality), then long cleaned title.
	base := full
	if i := strings.Index(base, " - "); i > 0 {
		base = strings.TrimSpace(base[:i])
	}
	if i := strings.Index(base, ":"); i > 0 {
		base = strings.TrimSpace(base[:i])
	}
	out := []string{base}
	if noSeason := trimCinemetaSeasonSuffix(base); noSeason != "" && !strings.EqualFold(noSeason, base) {
		out = append(out, noSeason)
	}
	if !strings.EqualFold(full, base) {
		out = append(out, full)
	}
	if noSeason := trimCinemetaSeasonSuffix(full); noSeason != "" && !strings.EqualFold(noSeason, full) {
		out = append(out, noSeason)
	}
	if n := normalizeForCinemeta(full); strings.Contains(n, "honzuki no gekokujou") {
		out = append(out,
			"Honzuki no Gekokujou",
			"Honzuki no Gekokujou Shisho ni Naru Tame ni wa Shudan wo Erandeiraremasen",
			"Ascendance of a Bookworm",
		)
	}
	uniq := make([]string, 0, len(out))
	for _, q := range out {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		found := false
		for _, e := range uniq {
			if strings.EqualFold(e, q) {
				found = true
				break
			}
		}
		if !found {
			uniq = append(uniq, q)
		}
	}
	return uniq
}

func scoreCinemetaCandidate(query, name string) float64 {
	q := normalizeForCinemeta(query)
	n := normalizeForCinemeta(name)
	if q == "" || n == "" {
		return 0
	}
	if q == n {
		return 100
	}
	if strings.Contains(n, q) || strings.Contains(q, n) {
		return 70
	}
	qTok := strings.Fields(q)
	if len(qTok) == 0 {
		return 0
	}
	set := map[string]struct{}{}
	for _, t := range strings.Fields(n) {
		set[t] = struct{}{}
	}
	hit := 0
	for _, t := range qTok {
		if _, ok := set[t]; ok {
			hit++
		}
	}
	return float64(hit) / float64(len(qTok)) * 60
}

func normalizeForCinemeta(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, ":", " ")
	s = strings.ReplaceAll(s, "-", " ")
	s = cinemetaSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func trimCinemetaSeasonSuffix(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for {
		next := strings.TrimSpace(cinemetaSeasonTail.ReplaceAllString(s, ""))
		if next == "" || strings.EqualFold(next, s) {
			break
		}
		s = next
	}
	return s
}

func extractStartYear(releaseInfo string) int {
	releaseInfo = strings.TrimSpace(releaseInfo)
	if releaseInfo == "" {
		return 0
	}
	digits := strings.Builder{}
	for _, r := range releaseInfo {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
			if digits.Len() == 4 {
				break
			}
		} else {
			if digits.Len() > 0 {
				break
			}
		}
	}
	if digits.Len() != 4 {
		return 0
	}
	yr, _ := strconv.Atoi(digits.String())
	if yr < 1900 || yr > time.Now().UTC().Year()+2 {
		return 0
	}
	return yr
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func (s *RSSSyncService) enrichTheTVDBGaps(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.SeriesEnrichment, syncNotes *[]string) {
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
		add := domain.SeriesEnrichment{TvdbSeriesID: tvdbID}
		if len(titles) > 0 {
			add.EpisodeTitleByNum = titles
		}
		if len(thumbs) > 0 {
			add.EpisodeThumbnailByNum = thumbs
		}
		for _, sid := range ids {
			cache[sid] = domain.MergeSeriesEnrichment(cache[sid], add)
			seriesUpdated++
		}
	}
	if fetches > 0 {
		s.log.Info("thetvdb: merged series id / episode maps", slog.Int("http_roundtrips", fetches), slog.Int("series_rows", seriesUpdated))
	}
}

func (s *RSSSyncService) translateEpisodeTitlesToPT(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.SeriesEnrichment, syncNotes *[]string) {
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

func (s *RSSSyncService) resolveStremioHeroBackgrounds(ctx context.Context, series []domain.CatalogSeries, cache map[string]domain.SeriesEnrichment, syncNotes *[]string) {
	if len(series) == 0 {
		return
	}
	if s.tmdb == nil {
		s.log.Info("tmdb: skipped (no API key or GOANIMES_TMDB_DISABLED; hero uses cached/TheTVDB candidates only)")
	}
	if s.tvdb == nil {
		s.log.Info("thetvdb: skipped (no GOANIMES_TVDB_API_KEY or GOANIMES_TVDB_DISABLED)")
	}
	if s.tmdb == nil && s.tvdb == nil {
		return
	}
	// TMDB/TheTVDB HTTP is expensive; reuse backdrop candidates for all series that share the same lookup key (IMDb, or MAL id).
	type tmdbFetchRep struct {
		en     domain.SeriesEnrichment
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
func heroTMDBFetchKey(en domain.SeriesEnrichment, seriesID string) string {
	if imdb := domain.NormalizeIMDbID(en.ImdbID); imdb != "" {
		return "imdb:" + imdb
	}
	if en.MalID > 0 {
		return fmt.Sprintf("mal:%d", en.MalID)
	}
	return "series:" + seriesID
}
