package stremio

import (
	"context"
	"math"
	"net/url"
	"sort"
	"regexp"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/ports"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	StremioType           = "anime"
	ManifestID            = "org.goanimes"
	ManifestName          = "GoAnimes"
	ManifestVersion       = "2.0.0"
	CatalogIDTrending     = "goanimes.trending"
	CatalogIDTopAiring    = "goanimes.top_airing"
	CatalogIDMostPopular  = "goanimes.most_popular"
	CatalogIDHighestRated = "goanimes.highest_rated"
)

type Service struct {
	Repo ports.CatalogRepository
	TMDB ports.TMDBClient
}

var tracer = otel.Tracer("goanimes/stremio")

var streamQualityBlockRe = regexp.MustCompile(`(?i)\[([^\]]*\b(?:480p|720p|1080p|2160p)\b[^\]]*)\]`)
var nonSlugCharsRe = regexp.MustCompile(`[^a-z0-9]+`)

func (s *Service) Manifest(ctx context.Context) (map[string]any, error) {
	ctx, span := tracer.Start(ctx, "stremio.manifest")
	defer span.End()
	genres, err := s.Repo.ListGenres(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		genres = []string{}
	}
	genreExtra := []map[string]any{{
		"name":       "genre",
		"isRequired": false,
		"options":    genres,
	}}
	return map[string]any{
		"id":          ManifestID,
		"version":     ManifestVersion,
		"name":        ManifestName,
		"logo":        "https://i.imgur.com/4Aq22Jp.jpeg",
		"background":  "https://i.imgur.com/YIqFCRR.jpeg",
		"description": "RSS anime torrents with curated mapping.",
		"types":       []string{StremioType},
		"genres":      genres,
		"catalogs": []map[string]any{
			{"type": StremioType, "id": CatalogIDTrending, "name": "GoAnimes · Em Alta", "extra": genreExtra},
			{"type": StremioType, "id": CatalogIDTopAiring, "name": "GoAnimes · Em Exibição", "extra": genreExtra},
			{"type": StremioType, "id": CatalogIDMostPopular, "name": "GoAnimes · Mais Populares", "extra": genreExtra},
			{"type": StremioType, "id": CatalogIDHighestRated, "name": "GoAnimes · Mais Bem Avaliados", "extra": genreExtra},
		},
		"resources":  []any{"catalog", "meta", "stream"},
		"idPrefixes": []string{domain.StremioIDPrefix},
	}, nil
}

func (s *Service) Catalog(ctx context.Context, catalogID string, extras map[string]string, limit, skip int) ([]map[string]any, error) {
	ctx, span := tracer.Start(ctx, "stremio.catalog")
	span.SetAttributes(
		attribute.String("catalog.id", catalogID),
		attribute.Int("catalog.limit", limit),
		attribute.Int("catalog.skip", skip),
	)
	defer span.End()
	animes, err := s.loadCatalogAnimes(ctx, catalogID, extras, limit, skip)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	metas := make([]map[string]any, 0, len(animes))
	for _, anime := range animes {
		metas = append(metas, map[string]any{
			"id":          domain.SeriesStremioID(anime.TMDBID, anime.SeasonNumber),
			"type":        StremioType,
			"name":        anime.Title,
			"poster":      anime.PosterPath,
			"genres":      anime.Genres,
			"description": anime.Overview,
		})
	}
	return metas, nil
}

func (s *Service) loadCatalogAnimes(ctx context.Context, catalogID string, extras map[string]string, limit, skip int) ([]domain.Anime, error) {
	switch catalogID {
	case CatalogIDTrending:
		return s.loadTrendingCatalog(ctx, extras, limit, skip)
	case CatalogIDTopAiring:
		return s.loadSortedCatalog(ctx, extras, limit, skip, filterCurrentAnime, compareTopAiring)
	case CatalogIDMostPopular:
		return s.loadSortedCatalog(ctx, extras, limit, skip, nil, compareMostPopular)
	case CatalogIDHighestRated:
		return s.loadSortedCatalog(ctx, extras, limit, skip, nil, compareHighestRated)
	default:
		return []domain.Anime{}, nil
	}
}

func (s *Service) loadTrendingCatalog(ctx context.Context, extras map[string]string, limit, skip int) ([]domain.Anime, error) {
	return s.loadSortedCatalog(ctx, extras, limit, skip, nil, compareTrending)
}

func (s *Service) loadSortedCatalog(ctx context.Context, extras map[string]string, limit, skip int, keep func(domain.Anime) bool, less func(domain.Anime, domain.Anime) bool) ([]domain.Anime, error) {
	animes, err := s.loadCatalogBase(ctx, extras)
	if err != nil {
		return nil, err
	}
	if keep != nil {
		filtered := make([]domain.Anime, 0, len(animes))
		for _, anime := range animes {
			if keep(anime) {
				filtered = append(filtered, anime)
			}
		}
		animes = filtered
	}
	sort.SliceStable(animes, func(i, j int) bool {
		return less(animes[i], animes[j])
	})
	animes = dedupeCatalogAnimes(animes)
	return paginateAnimes(animes, limit, skip), nil
}

func (s *Service) loadCatalogBase(ctx context.Context, extras map[string]string) ([]domain.Anime, error) {
	if g := strings.TrimSpace(extras["genre"]); g != "" {
		return s.Repo.ListByGenre(ctx, g, 200, 0)
	}
	return s.Repo.ListAll(ctx, 200, 0)
}

func paginateAnimes(animes []domain.Anime, limit, skip int) []domain.Anime {
	if skip < 0 {
		skip = 0
	}
	if skip >= len(animes) {
		return []domain.Anime{}
	}
	if limit <= 0 || limit > 200 {
		limit = 80
	}
	end := skip + limit
	if end > len(animes) {
		end = len(animes)
	}
	return animes[skip:end]
}

func dedupeCatalogAnimes(animes []domain.Anime) []domain.Anime {
	if len(animes) < 2 {
		return animes
	}
	seen := make(map[int]struct{}, len(animes))
	out := make([]domain.Anime, 0, len(animes))
	for _, anime := range animes {
		if _, ok := seen[anime.TMDBID]; ok {
			continue
		}
		seen[anime.TMDBID] = struct{}{}
		out = append(out, anime)
	}
	return out
}

func filterCurrentAnime(anime domain.Anime) bool {
	return strings.EqualFold(strings.TrimSpace(anime.Status), "current")
}

func compareTrending(a domain.Anime, b domain.Anime) bool {
	aScore := trendingScore(a)
	bScore := trendingScore(b)
	if aScore != bScore {
		return aScore > bScore
	}
	return compareTimesDesc(lastRelevantCatalogTime(a), lastRelevantCatalogTime(b), a, b)
}

func compareTopAiring(a domain.Anime, b domain.Anime) bool {
	aLatest := latestEpisodeReleaseAt(a)
	bLatest := latestEpisodeReleaseAt(b)
	if !aLatest.Equal(bLatest) {
		return aLatest.After(bLatest)
	}
	return compareTimesDesc(lastRelevantCatalogTime(a), lastRelevantCatalogTime(b), a, b)
}

func compareMostPopular(a domain.Anime, b domain.Anime) bool {
	aPopularity := popularityScore(a)
	bPopularity := popularityScore(b)
	if aPopularity != bPopularity {
		return aPopularity > bPopularity
	}
	if a.VoteCount != b.VoteCount {
		return a.VoteCount > b.VoteCount
	}
	return compareTimesDesc(lastRelevantCatalogTime(a), lastRelevantCatalogTime(b), a, b)
}

func compareHighestRated(a domain.Anime, b domain.Anime) bool {
	aScore := weightedRatingScore(a)
	bScore := weightedRatingScore(b)
	if aScore != bScore {
		return aScore > bScore
	}
	if a.Rating != b.Rating {
		return a.Rating > b.Rating
	}
	if a.VoteCount != b.VoteCount {
		return a.VoteCount > b.VoteCount
	}
	return compareTimesDesc(lastRelevantCatalogTime(a), lastRelevantCatalogTime(b), a, b)
}

func compareTimesDesc(aTime time.Time, bTime time.Time, a domain.Anime, b domain.Anime) bool {
	if !aTime.Equal(bTime) {
		return aTime.After(bTime)
	}
	if a.Title != b.Title {
		return a.Title < b.Title
	}
	if a.TMDBID != b.TMDBID {
		return a.TMDBID < b.TMDBID
	}
	return a.SeasonNumber < b.SeasonNumber
}

func latestEpisodeReleaseAt(anime domain.Anime) time.Time {
	lastEpisodeAt := parseDate(anime.LastEpisodeAt)
	nextEpisodeAt := parseDate(anime.NextEpisodeAt)
	if !nextEpisodeAt.IsZero() && sameOrBeforeToday(nextEpisodeAt) {
		return nextEpisodeAt
	}

	if !lastEpisodeAt.IsZero() {
		return lastEpisodeAt
	}
	return time.Time{}
}

func lastRelevantCatalogTime(anime domain.Anime) time.Time {
	if releasedAt := latestEpisodeReleaseAt(anime); !releasedAt.IsZero() {
		return releasedAt
	}
	return anime.UpdatedAt
}

func popularityScore(anime domain.Anime) float64 {
	score := anime.Popularity
	score += float64(sourceCount(anime)) * 0.25
	score += math.Min(float64(anime.VoteCount), 100) * 0.02
	return score
}

func sourceCount(anime domain.Anime) int {
	total := 0
	for _, ep := range anime.Episodes {
		total += len(ep.Sources)
	}
	return total
}

func trendingScore(anime domain.Anime) float64 {
	score := anime.Popularity
	if strings.EqualFold(strings.TrimSpace(anime.Status), "current") {
		score += 10
	}
	if strings.TrimSpace(anime.NextEpisodeAt) != "" {
		score += 8
	}
	if last := parseDate(anime.LastEpisodeAt); !last.IsZero() {
		days := time.Since(last).Hours() / 24
		switch {
		case days <= 7:
			score += 12
		case days <= 30:
			score += 6
		case days <= 90:
			score += 2
		}
	}
	score += math.Min(float64(anime.VoteCount), 100) * 0.03
	score += float64(sourceCount(anime)) * 0.15
	return score
}

func weightedRatingScore(anime domain.Anime) float64 {
	const minimumVotes = 25.0
	const baselineRating = 5.5
	votes := float64(anime.VoteCount)
	if votes <= 0 {
		return 0
	}
	return (votes/(votes+minimumVotes))*anime.Rating + (minimumVotes/(votes+minimumVotes))*baselineRating
}

func parseDate(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func sameOrBeforeToday(value time.Time) bool {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	candidate := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
	return !candidate.After(today)
}

func (s *Service) Meta(ctx context.Context, id string) (map[string]any, bool, error) {
	ctx, span := tracer.Start(ctx, "stremio.meta")
	span.SetAttributes(attribute.String("meta.id", id))
	defer span.End()
	tmdbID, season, ok := domain.ParseSeriesID(id)
	if !ok {
		return nil, false, nil
	}
	anime, found, err := s.Repo.GetByTMDBSeason(ctx, tmdbID, season)
	if err != nil || !found {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return nil, false, err
	}
	if needsMetaDetails(anime) && s.TMDB != nil {
		if details, detErr := s.TMDB.GetSeasonDetails(ctx, tmdbID, season); detErr == nil {
			changed := enrichAnimeFromTMDB(&anime, details)
			changed = applyLocalDerivedFields(&anime) || changed
			if changed {
				anime.UpdatedAt = time.Now().UTC()
				if upsertErr := s.Repo.UpsertSeason(ctx, anime); upsertErr != nil {
					span.RecordError(upsertErr)
				}
			}
		} else {
			span.RecordError(detErr)
		}
	} else if changed := applyLocalDerivedFields(&anime); changed {
		anime.UpdatedAt = time.Now().UTC()
		if upsertErr := s.Repo.UpsertSeason(ctx, anime); upsertErr != nil {
			span.RecordError(upsertErr)
		}
	}
	videos := make([]map[string]any, 0, len(anime.Episodes))
	for _, ep := range anime.Episodes {
		video := map[string]any{
			"id":       domain.EpisodeStremioID(anime.TMDBID, anime.SeasonNumber, ep.Number),
			"released": ep.AddedAt.Format(time.RFC3339),
			"season":   anime.SeasonNumber,
			"episode":  ep.Number,
		}
		// Use episode title if available, otherwise generate default
		if strings.TrimSpace(ep.Title) != "" {
			video["title"] = ep.Title
		} else {
			video["title"] = episodeTitle(ep.Number)
		}
		// Add episode image if available
		if strings.TrimSpace(ep.StillPath) != "" {
			video["thumbnail"] = ep.StillPath
		}
		// Add episode description if available
		if strings.TrimSpace(ep.Overview) != "" {
			video["overview"] = ep.Overview
		}
		videos = append(videos, video)
	}
	meta := map[string]any{
		"id":          domain.SeriesStremioID(anime.TMDBID, anime.SeasonNumber),
		"type":        StremioType,
		"animeType":   animeTypeOrDefault(anime.AnimeType),
		"name":        anime.Title,
		"slug":        anime.Slug,
		"aliases":     aliasesOrDefault(anime.Aliases, anime.Title),
		"logo":        anime.LogoPath,
		"poster":      anime.PosterPath,
		"genres":      anime.Genres,
		"releaseInfo": anime.ReleaseInfo,
		"year":        yearOrDefault(anime.Year, anime.ReleaseInfo),
		"status":      statusOrDefault(anime.Status),
		"runtime":     anime.Runtime,
		"rating":      anime.Rating,
		"description": func() string {
			if strings.TrimSpace(anime.Overview) != "" {
				return anime.Overview
			}
			return "Torrent releases with curated mapping."
		}(),
		"background": anime.BackdropPath,
		"videos":     videos,
	}
	return meta, true, nil
}

func needsMetaDetails(anime domain.Anime) bool {
	return strings.TrimSpace(anime.Overview) == "" ||
		strings.TrimSpace(anime.BackdropPath) == "" ||
		strings.TrimSpace(anime.PosterPath) == "" ||
		strings.TrimSpace(anime.Title) == "" ||
		len(anime.Genres) == 0 ||
		anime.Rating == 0 ||
		strings.TrimSpace(anime.Slug) == "" ||
		strings.TrimSpace(anime.AnimeType) == "" ||
		strings.TrimSpace(anime.ReleaseInfo) == "" ||
		strings.TrimSpace(anime.Year) == "" ||
		strings.TrimSpace(anime.Status) == "" ||
		strings.TrimSpace(anime.Runtime) == "" ||
		len(anime.Aliases) == 0
}

func enrichAnimeFromTMDB(anime *domain.Anime, details ports.TMDBSeasonDetails) bool {
	if anime == nil {
		return false
	}
	changed := false
	if strings.TrimSpace(anime.Title) == "" && strings.TrimSpace(details.Title) != "" {
		anime.Title = strings.TrimSpace(details.Title)
		changed = true
	}
	if strings.TrimSpace(anime.Overview) == "" && strings.TrimSpace(details.Overview) != "" {
		anime.Overview = strings.TrimSpace(details.Overview)
		changed = true
	}
	if strings.TrimSpace(anime.PosterPath) == "" && strings.TrimSpace(details.PosterPath) != "" {
		anime.PosterPath = strings.TrimSpace(details.PosterPath)
		changed = true
	}
	if strings.TrimSpace(anime.BackdropPath) == "" && strings.TrimSpace(details.BackdropPath) != "" {
		anime.BackdropPath = strings.TrimSpace(details.BackdropPath)
		changed = true
	}
	if len(anime.Genres) == 0 && len(details.Genres) > 0 {
		anime.Genres = details.Genres
		changed = true
	}
	if anime.Rating == 0 && details.Rating > 0 {
		anime.Rating = details.Rating
		changed = true
	}
	if anime.VoteCount == 0 && details.VoteCount > 0 {
		anime.VoteCount = details.VoteCount
		changed = true
	}
	if anime.Popularity == 0 && details.Popularity > 0 {
		anime.Popularity = details.Popularity
		changed = true
	}
	if strings.TrimSpace(anime.LastEpisodeAt) == "" && strings.TrimSpace(details.LastEpisodeAirDate) != "" {
		anime.LastEpisodeAt = strings.TrimSpace(details.LastEpisodeAirDate)
		changed = true
	}
	if anime.LastEpisodeNo == 0 && details.LastEpisodeNumber > 0 {
		anime.LastEpisodeNo = details.LastEpisodeNumber
		changed = true
	}
	if strings.TrimSpace(anime.NextEpisodeAt) == "" && strings.TrimSpace(details.NextEpisodeAirDate) != "" {
		anime.NextEpisodeAt = strings.TrimSpace(details.NextEpisodeAirDate)
		changed = true
	}
	if anime.NextEpisodeNo == 0 && details.NextEpisodeNumber > 0 {
		anime.NextEpisodeNo = details.NextEpisodeNumber
		changed = true
	}

	derivedAnimeType := normalizeAnimeType(details.TVType)
	if strings.TrimSpace(anime.AnimeType) == "" && derivedAnimeType != "" {
		anime.AnimeType = derivedAnimeType
		changed = true
	}
	derivedSlug := slugify(anime.Title)
	if strings.TrimSpace(anime.Slug) == "" && derivedSlug != "" {
		anime.Slug = derivedSlug
		changed = true
	}
	if len(anime.Aliases) == 0 {
		aliases := buildAliases(anime.Title, details.OriginalTitle)
		if len(aliases) > 0 {
			anime.Aliases = aliases
			changed = true
		}
	}
	releaseInfo, year := deriveReleaseInfo(details.FirstAirDate, details.LastAirDate, details.Status, details.InProduction, details.HasNextEpisode)
	if strings.TrimSpace(anime.ReleaseInfo) == "" && releaseInfo != "" {
		anime.ReleaseInfo = releaseInfo
		changed = true
	}
	if strings.TrimSpace(anime.Year) == "" && year != "" {
		anime.Year = year
		changed = true
	}
	derivedStatus := deriveCatalogStatus(details.Status, details.InProduction, details.HasNextEpisode)
	if strings.TrimSpace(anime.Status) == "" && derivedStatus != "" {
		anime.Status = derivedStatus
		changed = true
	}
	derivedRuntime := deriveRuntime(details.EpisodeRunTime, details.SeasonRunTime)
	if strings.TrimSpace(anime.Runtime) == "" && derivedRuntime != "" {
		anime.Runtime = derivedRuntime
		changed = true
	}
	return changed
}

func applyLocalDerivedFields(anime *domain.Anime) bool {
	if anime == nil {
		return false
	}
	changed := false
	if strings.TrimSpace(anime.AnimeType) == "" {
		anime.AnimeType = "TV"
		changed = true
	}
	if strings.TrimSpace(anime.Slug) == "" {
		slug := slugify(anime.Title)
		if slug != "" {
			anime.Slug = slug
			changed = true
		}
	}
	if len(anime.Aliases) == 0 {
		aliases := buildAliases(anime.Title, "")
		if len(aliases) > 0 {
			anime.Aliases = aliases
			changed = true
		}
	}
	if strings.TrimSpace(anime.Status) == "" {
		anime.Status = "unknown"
		changed = true
	}
	if strings.TrimSpace(anime.Year) == "" {
		if y := yearOrDefault("", anime.ReleaseInfo); y != "" {
			anime.Year = y
			changed = true
		}
	}
	return changed
}

func normalizeAnimeType(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "TV"
	}
	return "TV"
}

func animeTypeOrDefault(v string) string {
	if strings.TrimSpace(v) == "" {
		return "TV"
	}
	return strings.TrimSpace(v)
}

func aliasesOrDefault(aliases []string, title string) []string {
	if len(aliases) > 0 {
		return aliases
	}
	if strings.TrimSpace(title) == "" {
		return []string{}
	}
	return []string{strings.TrimSpace(title)}
}

func buildAliases(values ...string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		key := strings.ToLower(normalized)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func statusOrDefault(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "unknown"
	}
	return status
}

func deriveCatalogStatus(tmdbStatus string, inProduction bool, hasNextEpisode bool) string {
	isAiring := strings.EqualFold(strings.TrimSpace(tmdbStatus), "Returning Series") || inProduction
	if isAiring || hasNextEpisode {
		return "current"
	}
	if strings.EqualFold(strings.TrimSpace(tmdbStatus), "Ended") {
		return "ended"
	}
	return "unknown"
}

func deriveReleaseInfo(firstAirDate, lastAirDate, tmdbStatus string, inProduction bool, hasNextEpisode bool) (string, string) {
	startYear := yearFromDate(firstAirDate)
	if startYear == "" {
		return "", ""
	}
	status := deriveCatalogStatus(tmdbStatus, inProduction, hasNextEpisode)
	if status == "current" {
		return startYear + "-", startYear + "-"
	}
	if status == "ended" {
		endYear := yearFromDate(lastAirDate)
		if endYear != "" && endYear != startYear {
			return startYear + "-" + endYear, startYear + "-" + endYear
		}
		return startYear, startYear
	}
	return startYear, startYear
}

func yearFromDate(v string) string {
	v = strings.TrimSpace(v)
	if len(v) < 4 {
		return ""
	}
	return v[:4]
}

func yearOrDefault(year string, releaseInfo string) string {
	year = strings.TrimSpace(year)
	if year != "" {
		return year
	}
	releaseInfo = strings.TrimSpace(releaseInfo)
	return releaseInfo
}

func deriveRuntime(showRuntime []int, seasonRuntime []int) string {
	for _, runtime := range showRuntime {
		if runtime > 0 {
			return itoa(runtime) + " min"
		}
	}
	for _, runtime := range seasonRuntime {
		if runtime > 0 {
			return itoa(runtime) + " min"
		}
	}
	return ""
}

func slugify(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"ç", "c", "ñ", "n",
	)
	value = replacer.Replace(value)
	value = nonSlugCharsRe.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	return value
}

func (s *Service) Streams(ctx context.Context, id string) ([]map[string]any, error) {
	ctx, span := tracer.Start(ctx, "stremio.streams")
	span.SetAttributes(attribute.String("stream.id", id))
	defer span.End()
	tmdbID, season, episode, ok := domain.ParseEpisodeID(id)
	if !ok {
		return []map[string]any{}, nil
	}
	anime, found, err := s.Repo.GetByTMDBSeason(ctx, tmdbID, season)
	if err != nil || !found {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return []map[string]any{}, err
	}
	for _, ep := range anime.Episodes {
		if ep.Number != episode {
			continue
		}
		streams := make([]map[string]any, 0, len(ep.Sources))
		for _, src := range ep.Sources {
			magnet, err := resolvePlaybackURL(ctx, src.MagnetLink)
			if err != nil || strings.TrimSpace(magnet) == "" {
				continue
			}
			infoHash := magnetInfoHash(magnet)
			if strings.TrimSpace(infoHash) == "" {
				continue
			}
			streams = append(streams, map[string]any{
				"behaviorHints": map[string]any{
					"bingeGroup": id,
				},
				"fileIdx":  0,
				"infoHash": infoHash,
				"name":     buildStreamName(src.Quality, magnet),
				"title":    streamTitle(episode, magnet),
			})
		}
		return streams, nil
	}
	return []map[string]any{}, nil
}

func parseCatalogExtras(catalogPath string) (string, map[string]string, bool) {
	p := strings.TrimPrefix(strings.TrimSpace(catalogPath), "/")
	if p == "" {
		return "", nil, false
	}
	p = strings.TrimSuffix(p, ".json")
	parts := strings.Split(p, "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", nil, false
	}
	extras := make(map[string]string)
	for _, seg := range parts[1:] {
		k, v, has := strings.Cut(seg, "=")
		if !has {
			continue
		}
		key, errK := url.PathUnescape(strings.TrimSpace(k))
		val, errV := url.PathUnescape(strings.TrimSpace(v))
		if errK != nil || errV != nil || key == "" {
			continue
		}
		extras[key] = val
	}
	return parts[0], extras, true
}

func episodeTitle(ep int) string {
	return "Episódio " + itoa(ep)
}

func buildStreamName(quality string, magnet string) string {
	name := "Torrent"
	if resolution := extractResolution(quality, magnet); strings.TrimSpace(resolution) != "" {
		name = name + " · " + strings.TrimSpace(resolution)
	}
	return name
}

func extractResolution(quality string, magnet string) string {
	fullQuality := ""
	if q := normalizedStreamQuality(quality); q != "" {
		fullQuality = q
	} else if q := qualityFromLink(magnet); q != "" {
		fullQuality = q
	}
	if fullQuality == "" {
		return ""
	}
	// Extract just the resolution (first word usually contains it)
	parts := strings.Fields(fullQuality)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func streamTitle(ep int, rawLink string) string {
	name := releaseNameFromLink(rawLink)
	if strings.TrimSpace(name) == "" {
		return episodeTitle(ep)
	}
	return episodeTitle(ep) + " · [Torrent] " + name
}

func normalizedStreamQuality(quality string) string {
	quality = strings.TrimSpace(quality)
	if quality == "" {
		return ""
	}
	return quality
}

func releaseNameFromLink(rawLink string) string {
	rawLink = strings.TrimSpace(rawLink)
	if rawLink == "" {
		return ""
	}
	decoded := rawLink
	if parsed, err := url.Parse(rawLink); err == nil {
		if strings.EqualFold(parsed.Scheme, "magnet") {
			if dn := strings.TrimSpace(parsed.Query().Get("dn")); dn != "" {
				if unescaped, err := url.QueryUnescape(dn); err == nil {
					decoded = unescaped
				} else {
					decoded = dn
				}
			}
		} else if parsed.Path != "" {
			if unescaped, err := url.PathUnescape(parsed.Path); err == nil {
				decoded = unescaped
			} else {
				decoded = parsed.Path
			}
		}
	}
	return decoded
}

func qualityFromLink(rawLink string) string {
	rawLink = strings.TrimSpace(rawLink)
	if rawLink == "" {
		return ""
	}
	decoded := rawLink
	if parsed, err := url.Parse(rawLink); err == nil {
		if strings.EqualFold(parsed.Scheme, "magnet") {
			if dn := strings.TrimSpace(parsed.Query().Get("dn")); dn != "" {
				if unescaped, err := url.QueryUnescape(dn); err == nil {
					decoded = unescaped
				} else {
					decoded = dn
				}
			}
		} else if parsed.Path != "" {
			if unescaped, err := url.PathUnescape(parsed.Path); err == nil {
				decoded = unescaped
			} else {
				decoded = parsed.Path
			}
		}
	}
	if match := streamQualityBlockRe.FindStringSubmatch(decoded); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func magnetInfoHash(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(parsed.Scheme, "magnet") {
		return ""
	}
	xt := strings.TrimSpace(parsed.Query().Get("xt"))
	if xt == "" {
		return ""
	}
	const prefix = "urn:btih:"
	if !strings.HasPrefix(strings.ToLower(xt), prefix) {
		return ""
	}
	h := strings.TrimSpace(xt[len(prefix):])
	if h == "" {
		return ""
	}
	return strings.ToLower(h)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 12)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	if neg {
		buf = append(buf, '-')
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

func ParseCatalogPath(path string) (string, map[string]string, bool) {
	return parseCatalogExtras(path)
}
