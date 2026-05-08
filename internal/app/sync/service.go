package sync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/ports"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type Service struct {
	Feeds   ports.FeedRepository
	Mapping ports.MappingRepository
	Catalog ports.CatalogRepository
	Reader  ports.FeedReader
	TMDB    ports.TMDBClient
	Guard   *Guard
}

var tracer = otel.Tracer("goanimes/sync")

func (s *Service) Run(ctx context.Context) Result {
	res := s.startResult()
	if s.Guard != nil && !s.Guard.TryStart() {
		_, span := tracer.Start(ctx, "sync.run")
		defer span.End()
		err := errors.New("sync already running")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return s.finishResult(res, err)
	}
	defer func() {
		if s.Guard != nil {
			s.Guard.Finish()
		}
	}()
	return s.runInner(ctx)
}

// runInner executes one sync run; Run and RequestAsync hold the Guard when used.
func (s *Service) runInner(ctx context.Context) Result {
	ctx, span := tracer.Start(ctx, "sync.run")
	defer span.End()
	res := s.startResult()
	defer func() {
		res.FinishedAt = time.Now().UTC()
	}()

	feeds, err := s.Feeds.ListEnabled(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		res.Errors = append(res.Errors, err)
		return res
	}
	for _, feed := range feeds {
		res = s.processFeed(ctx, res, feed)
	}
	return res
}

func (s *Service) ForceRun(ctx context.Context) Result {
	res := s.runInner(ctx)
	if s.TMDB == nil || s.Catalog == nil {
		return res
	}
	if err := s.backfillCatalog(ctx); err != nil {
		res.Errors = append(res.Errors, err)
	}
	return res
}

// RequestAsync runs a sync in the background if none is running (same mutex as Run).
// Returns false if a sync is already in progress. Uses context.Background so work is not
// cancelled when the HTTP client disconnects.
func (s *Service) RequestAsync(force bool) bool {
	// Note: this method intentionally starts background work using
	// `context.Background()` so the started sync is not cancelled when the
	// originating HTTP request context is finished. This means the background
	// sync run will be a root trace (no parent) unless a caller explicitly
	// changes this behavior by passing a context through a different API.
	//
	// If you want the spawned goroutine to inherit the caller's trace span,
	// change the API to accept a `context.Context` and propagate it into the
	// goroutine (e.g. `go func(ctx context.Context) { _ = s.runInner(ctx) }(ctx)`).
	if s.Guard == nil {
		go func() {
			if force {
				_ = s.ForceRun(context.Background())
				return
			}
			_ = s.runInner(context.Background())
		}()
		return true
	}
	if !s.Guard.TryStart() {
		return false
	}
	go func() {
		defer s.Guard.Finish()
		if force {
			_ = s.ForceRun(context.Background())
			return
		}
		_ = s.runInner(context.Background())
	}()
	return true
}

func (s *Service) startResult() Result {
	return Result{StartedAt: time.Now().UTC()}
}

func (s *Service) finishResult(res Result, err error) Result {
	if err != nil {
		res.Errors = append(res.Errors, err)
	}
	res.FinishedAt = time.Now().UTC()
	return res
}

func (s *Service) processFeed(ctx context.Context, res Result, feed domain.Feed) Result {
	ctx, span := tracer.Start(ctx, "sync.process_feed")
	span.SetAttributes(
		attribute.String("feed.id", feed.ID),
		attribute.String("feed.name", feed.Name),
		attribute.String("feed.type", string(feed.Type)),
	)
	defer span.End()
	items, fetchErr := s.Reader.Fetch(ctx, feed)
	if fetchErr != nil {
		span.RecordError(fetchErr)
		span.SetStatus(codes.Error, fetchErr.Error())
		res.Errors = append(res.Errors, fetchErr)
		return res
	}
	for _, item := range items {
		res = s.processItem(ctx, res, item)
	}
	return res
}

func (s *Service) processItem(ctx context.Context, res Result, item ports.ReleaseItem) Result {
	norm := normalizeItem(item)
	ctx, span := tracer.Start(ctx, "sync.process_item")
	span.SetAttributes(
		attribute.String("release.provider", norm.Provider),
		attribute.String("release.key", norm.RSSNameKey),
		attribute.Int("release.episode", norm.Episode),
	)
	defer span.End()
	if !s.isValidNormalized(norm) {
		return s.addUnmatched(ctx, res, norm, item)
	}
	mapping, ok, err := s.Mapping.FindOverride(ctx, norm.RSSNameKey)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		res.Errors = append(res.Errors, err)
		return res
	}
	tmdbID, season, mappedEpisode, ok, err := s.resolveMapping(ctx, norm, item, mapping, ok)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		res.Errors = append(res.Errors, err)
		return res
	}
	if !ok || tmdbID <= 0 || season <= 0 {
		return res
	}
	addedErr := s.addEpisodeSource(ctx, tmdbID, season, mappedEpisode, norm)
	if addedErr != nil {
		span.RecordError(addedErr)
		span.SetStatus(codes.Error, addedErr.Error())
		res.Errors = append(res.Errors, addedErr)
		return res
	}
	// Fetch episode details from TMDB if available
	_ = s.enrichEpisodeDetails(ctx, tmdbID, season, mappedEpisode)
	if ensureErr := s.ensureSeason(ctx, tmdbID, season, norm); ensureErr != nil {
		span.RecordError(ensureErr)
		span.SetStatus(codes.Error, ensureErr.Error())
		res.Errors = append(res.Errors, ensureErr)
		return res
	}
	res.Processed++
	return res
}

func (s *Service) isValidNormalized(norm NormalizedRelease) bool {
	return norm.RSSNameKey != "" && norm.Episode > 0
}

func (s *Service) addUnmatched(ctx context.Context, res Result, norm NormalizedRelease, item ports.ReleaseItem) Result {
	_ = s.Mapping.AddUnmatched(ctx, domain.UnmatchedRelease{
		RSSNameKey: norm.RSSNameKey,
		RawTitle:   item.Title,
		Provider:   item.Provider,
		AddedAt:    time.Now().UTC(),
		LastSeenAt: time.Now().UTC(),
		Count:      1,
	})
	return res
}

func (s *Service) resolveMapping(
	ctx context.Context,
	norm NormalizedRelease,
	item ports.ReleaseItem,
	override domain.MappingOverride,
	matched bool,
) (tmdbID int, season int, mappedEpisode int, ok bool, err error) {
	// Default mappedEpisode is the normalized episode
	mappedEpisode = norm.Episode
	if matched {
		// If override contains an episode offset, apply it
		if override.EpisodeOffset > 0 {
			mappedEpisode = norm.Episode + override.EpisodeOffset
		}
		return override.TMDBID, override.Season, mappedEpisode, true, nil
	}
	if s.TMDB == nil {
		s.addUnmatched(ctx, Result{}, norm, item)
		return 0, 0, 0, false, nil
	}
	search, found, searchErr := s.TMDB.SearchSeries(ctx, norm.RSSNameKey)
	if searchErr != nil {
		return 0, 0, 0, false, searchErr
	}
	if !found {
		s.addUnmatched(ctx, Result{}, norm, item)
		return 0, 0, 0, false, nil
	}
	return search.TMDBID, 1, mappedEpisode, true, nil
}

func (s *Service) addEpisodeSource(ctx context.Context, tmdbID, season, episode int, norm NormalizedRelease) error {
	_, addErr := s.Catalog.AddEpisodeSource(ctx, tmdbID, season, episode, domain.Source{
		Provider:   norm.Provider,
		MagnetLink: norm.MagnetLink,
		Quality:    norm.Quality,
	})
	return addErr
}

func (s *Service) enrichEpisodeDetails(ctx context.Context, tmdbID, season, episodeNum int) error {
	if s.TMDB == nil {
		return nil
	}
	episodeDetails, err := s.TMDB.GetEpisodeDetails(ctx, tmdbID, season, episodeNum)
	if err != nil {
		return nil // Non-blocking error, continue on TMDB failure
	}
	// Update episode with TMDB details
	return s.Catalog.UpdateEpisodeDetails(ctx, tmdbID, season, episodeNum, episodeDetails.Title, episodeDetails.Overview, episodeDetails.StillPath)
}

func (s *Service) ensureSeason(ctx context.Context, tmdbID, season int, norm NormalizedRelease) error {
	existing, found, _ := s.Catalog.GetByTMDBSeason(ctx, tmdbID, season)

	anime := existing
	if !found {
		anime = domain.Anime{
			TMDBID:        tmdbID,
			SeasonNumber:  season,
			Title:         norm.RSSNameKey,
			MappingStatus: domain.MappingStatusMapped,
			UpdatedAt:     time.Now().UTC(),
		}
	}

	// Fetch TMDB details if available and fill in empty fields
	needsUpdate := !found
	if s.TMDB != nil {
		details, detErr := s.TMDB.GetSeasonDetails(ctx, tmdbID, season)
		if detErr != nil {
			// If anime doesn't exist yet, return the error; if it exists, continue
			if !found {
				return detErr
			}
		} else {
			// Fill empty fields with TMDB details
			if anime.Title == "" || anime.Title == norm.RSSNameKey {
				anime.Title = details.Title
				needsUpdate = true
			}
			if anime.AnimeType == "" {
				anime.AnimeType = normalizeAnimeType(details.TVType)
				needsUpdate = true
			}
			if anime.Slug == "" {
				if slug := slugify(anime.Title); slug != "" {
					anime.Slug = slug
					needsUpdate = true
				}
			}
			if len(anime.Aliases) == 0 {
				if aliases := buildAliases(anime.Title, details.OriginalTitle); len(aliases) > 0 {
					anime.Aliases = aliases
					needsUpdate = true
				}
			}
			if anime.Overview == "" && details.Overview != "" {
				anime.Overview = details.Overview
				needsUpdate = true
			}
			if len(anime.Genres) == 0 && len(details.Genres) > 0 {
				anime.Genres = details.Genres
				needsUpdate = true
			}
			if anime.Rating == 0 && details.Rating > 0 {
				anime.Rating = details.Rating
				needsUpdate = true
			}
			if anime.VoteCount == 0 && details.VoteCount > 0 {
				anime.VoteCount = details.VoteCount
				needsUpdate = true
			}
			if anime.Popularity == 0 && details.Popularity > 0 {
				anime.Popularity = details.Popularity
				needsUpdate = true
			}
			if anime.PosterPath == "" && details.PosterPath != "" {
				anime.PosterPath = details.PosterPath
				needsUpdate = true
			}
			if anime.BackdropPath == "" && details.BackdropPath != "" {
				anime.BackdropPath = details.BackdropPath
				needsUpdate = true
			}
			if anime.LastEpisodeAt == "" && details.LastEpisodeAirDate != "" {
				anime.LastEpisodeAt = details.LastEpisodeAirDate
				needsUpdate = true
			}
			if anime.LastEpisodeNo == 0 && details.LastEpisodeNumber > 0 {
				anime.LastEpisodeNo = details.LastEpisodeNumber
				needsUpdate = true
			}
			if anime.NextEpisodeAt == "" && details.NextEpisodeAirDate != "" {
				anime.NextEpisodeAt = details.NextEpisodeAirDate
				needsUpdate = true
			}
			if anime.NextEpisodeNo == 0 && details.NextEpisodeNumber > 0 {
				anime.NextEpisodeNo = details.NextEpisodeNumber
				needsUpdate = true
			}
			releaseInfo, year := deriveReleaseInfo(details.FirstAirDate, details.LastAirDate, details.Status, details.InProduction, details.HasNextEpisode)
			if anime.ReleaseInfo == "" && releaseInfo != "" {
				anime.ReleaseInfo = releaseInfo
				needsUpdate = true
			}
			if anime.Year == "" && year != "" {
				anime.Year = year
				needsUpdate = true
			}
			if anime.Status == "" {
				anime.Status = deriveCatalogStatus(details.Status, details.InProduction, details.HasNextEpisode)
				needsUpdate = true
			}
			if anime.Runtime == "" {
				if runtime := deriveRuntime(details.EpisodeRunTime, details.SeasonRunTime); runtime != "" {
					anime.Runtime = runtime
					needsUpdate = true
				}
			}
		}
	}

	if needsUpdate {
		anime.UpdatedAt = time.Now().UTC()
		return s.Catalog.UpsertSeason(ctx, anime)
	}
	return nil
}

func (s *Service) backfillCatalog(ctx context.Context) error {
	const pageSize = 200
	for skip := 0; ; skip += pageSize {
		animes, err := s.Catalog.ListAll(ctx, pageSize, skip)
		if err != nil {
			return err
		}
		for _, anime := range animes {
			updated, changed, err := s.refreshAnimeDetails(ctx, anime)
			if err != nil {
				return err
			}
			if changed {
				if err := s.Catalog.UpsertSeason(ctx, updated); err != nil {
					return err
				}
			}
		}
		if len(animes) < pageSize {
			return nil
		}
	}
}

func (s *Service) refreshAnimeDetails(ctx context.Context, anime domain.Anime) (domain.Anime, bool, error) {
	if s.TMDB == nil {
		return anime, false, nil
	}
	details, err := s.TMDB.GetSeasonDetails(ctx, anime.TMDBID, anime.SeasonNumber)
	if err != nil {
		return anime, false, err
	}
	changed := false
	if title := strings.TrimSpace(details.Title); title != "" && anime.Title != title {
		anime.Title = title
		changed = true
	}
	animeType := normalizeAnimeType(details.TVType)
	if animeType != "" && strings.TrimSpace(anime.AnimeType) != animeType {
		anime.AnimeType = animeType
		changed = true
	}
	if slug := slugify(anime.Title); slug != "" && strings.TrimSpace(anime.Slug) != slug {
		anime.Slug = slug
		changed = true
	}
	if aliases := buildAliases(anime.Title, details.OriginalTitle); !equalStrings(anime.Aliases, aliases) {
		anime.Aliases = aliases
		changed = true
	}
	if overview := strings.TrimSpace(details.Overview); overview != "" && strings.TrimSpace(anime.Overview) != overview {
		anime.Overview = overview
		changed = true
	}
	if len(details.Genres) > 0 && !equalStrings(anime.Genres, details.Genres) {
		anime.Genres = details.Genres
		changed = true
	}
	if details.Rating > 0 && anime.Rating != details.Rating {
		anime.Rating = details.Rating
		changed = true
	}
	if details.VoteCount > 0 && anime.VoteCount != details.VoteCount {
		anime.VoteCount = details.VoteCount
		changed = true
	}
	if details.Popularity > 0 && anime.Popularity != details.Popularity {
		anime.Popularity = details.Popularity
		changed = true
	}
	if poster := strings.TrimSpace(details.PosterPath); poster != "" && strings.TrimSpace(anime.PosterPath) != poster {
		anime.PosterPath = poster
		changed = true
	}
	if backdrop := strings.TrimSpace(details.BackdropPath); backdrop != "" && strings.TrimSpace(anime.BackdropPath) != backdrop {
		anime.BackdropPath = backdrop
		changed = true
	}
	if lastAt := strings.TrimSpace(details.LastEpisodeAirDate); lastAt != "" && strings.TrimSpace(anime.LastEpisodeAt) != lastAt {
		anime.LastEpisodeAt = lastAt
		changed = true
	}
	if details.LastEpisodeNumber > 0 && anime.LastEpisodeNo != details.LastEpisodeNumber {
		anime.LastEpisodeNo = details.LastEpisodeNumber
		changed = true
	}
	nextAt := strings.TrimSpace(details.NextEpisodeAirDate)
	if strings.TrimSpace(anime.NextEpisodeAt) != nextAt {
		anime.NextEpisodeAt = nextAt
		changed = true
	}
	if anime.NextEpisodeNo != details.NextEpisodeNumber {
		anime.NextEpisodeNo = details.NextEpisodeNumber
		changed = true
	}
	releaseInfo, year := deriveReleaseInfo(details.FirstAirDate, details.LastAirDate, details.Status, details.InProduction, details.HasNextEpisode)
	if strings.TrimSpace(anime.ReleaseInfo) != releaseInfo {
		anime.ReleaseInfo = releaseInfo
		changed = true
	}
	if strings.TrimSpace(anime.Year) != year {
		anime.Year = year
		changed = true
	}
	status := deriveCatalogStatus(details.Status, details.InProduction, details.HasNextEpisode)
	if strings.TrimSpace(anime.Status) != status {
		anime.Status = status
		changed = true
	}
	runtime := deriveRuntime(details.EpisodeRunTime, details.SeasonRunTime)
	if strings.TrimSpace(anime.Runtime) != runtime {
		anime.Runtime = runtime
		changed = true
	}
	if changed {
		anime.UpdatedAt = time.Now().UTC()
	}
	return anime, changed, nil
}

func fillMissingAnimeDetailsFromTMDB(anime domain.Anime, details ports.TMDBSeasonDetails) (domain.Anime, bool) {
	changed := false
	if strings.TrimSpace(anime.Title) == "" {
		anime.Title = details.Title
		changed = true
	}
	if strings.TrimSpace(anime.AnimeType) == "" {
		anime.AnimeType = normalizeAnimeType(details.TVType)
		changed = true
	}
	if strings.TrimSpace(anime.Slug) == "" {
		if slug := slugify(anime.Title); slug != "" {
			anime.Slug = slug
			changed = true
		}
	}
	if len(anime.Aliases) == 0 {
		if aliases := buildAliases(anime.Title, details.OriginalTitle); len(aliases) > 0 {
			anime.Aliases = aliases
			changed = true
		}
	}
	if strings.TrimSpace(anime.Overview) == "" && strings.TrimSpace(details.Overview) != "" {
		anime.Overview = details.Overview
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
	if strings.TrimSpace(anime.PosterPath) == "" && strings.TrimSpace(details.PosterPath) != "" {
		anime.PosterPath = details.PosterPath
		changed = true
	}
	if strings.TrimSpace(anime.BackdropPath) == "" && strings.TrimSpace(details.BackdropPath) != "" {
		anime.BackdropPath = details.BackdropPath
		changed = true
	}
	if strings.TrimSpace(anime.LastEpisodeAt) == "" && strings.TrimSpace(details.LastEpisodeAirDate) != "" {
		anime.LastEpisodeAt = details.LastEpisodeAirDate
		changed = true
	}
	if anime.LastEpisodeNo == 0 && details.LastEpisodeNumber > 0 {
		anime.LastEpisodeNo = details.LastEpisodeNumber
		changed = true
	}
	if strings.TrimSpace(anime.NextEpisodeAt) == "" && strings.TrimSpace(details.NextEpisodeAirDate) != "" {
		anime.NextEpisodeAt = details.NextEpisodeAirDate
		changed = true
	}
	if anime.NextEpisodeNo == 0 && details.NextEpisodeNumber > 0 {
		anime.NextEpisodeNo = details.NextEpisodeNumber
		changed = true
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
	if strings.TrimSpace(anime.Status) == "" {
		anime.Status = deriveCatalogStatus(details.Status, details.InProduction, details.HasNextEpisode)
		changed = true
	}
	if strings.TrimSpace(anime.Runtime) == "" {
		if runtime := deriveRuntime(details.EpisodeRunTime, details.SeasonRunTime); runtime != "" {
			anime.Runtime = runtime
			changed = true
		}
	}
	if changed {
		anime.UpdatedAt = time.Now().UTC()
	}
	return anime, changed
}

func needsAnimeDetails(anime domain.Anime) bool {
	return strings.TrimSpace(anime.Overview) == "" ||
		strings.TrimSpace(anime.BackdropPath) == "" ||
		strings.TrimSpace(anime.PosterPath) == "" ||
		strings.TrimSpace(anime.Title) == "" ||
		len(anime.Genres) == 0 ||
		anime.Rating == 0 ||
		anime.VoteCount == 0 ||
		anime.Popularity == 0 ||
		strings.TrimSpace(anime.AnimeType) == "" ||
		strings.TrimSpace(anime.Slug) == "" ||
		len(anime.Aliases) == 0 ||
		strings.TrimSpace(anime.ReleaseInfo) == "" ||
		strings.TrimSpace(anime.Year) == "" ||
		strings.TrimSpace(anime.Status) == "" ||
		strings.TrimSpace(anime.Runtime) == "" ||
		strings.TrimSpace(anime.LastEpisodeAt) == "" ||
		anime.LastEpisodeNo == 0 ||
		(anime.NextEpisodeAt != "" && anime.NextEpisodeNo == 0)
}

func normalizeAnimeType(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "TV"
	}
	return "TV"
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
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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

func deriveRuntime(showRuntime []int, seasonRuntime []int) string {
	for _, runtime := range showRuntime {
		if runtime > 0 {
			return fmt.Sprintf("%d min", runtime)
		}
	}
	for _, runtime := range seasonRuntime {
		if runtime > 0 {
			return fmt.Sprintf("%d min", runtime)
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
	slug := make([]rune, 0, len(value))
	lastDash := false
	for _, r := range value {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlnum {
			slug = append(slug, r)
			lastDash = false
			continue
		}
		if !lastDash {
			slug = append(slug, '-')
			lastDash = true
		}
	}
	return strings.Trim(string(slug), "-")
}

func normalizeItem(item ports.ReleaseItem) NormalizedRelease {
	key, ep, quality := NormalizeTitle(item.Title)
	magnet := item.Magnet
	if magnet == "" {
		magnet = item.Link
	}
	return NormalizedRelease{
		RSSNameKey: key,
		Title:      item.Title,
		Episode:    ep,
		Quality:    quality,
		MagnetLink: magnet,
		Provider:   item.Provider,
		Published:  item.Published,
	}
}
