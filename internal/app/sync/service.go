package sync

import (
	"context"
	"errors"
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

// RequestAsync runs a sync in the background if none is running (same mutex as Run).
// Returns false if a sync is already in progress. Uses context.Background so work is not
// cancelled when the HTTP client disconnects.
func (s *Service) RequestAsync() bool {
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
		go func() { _ = s.runInner(context.Background()) }()
		return true
	}
	if !s.Guard.TryStart() {
		return false
	}
	go func() {
		defer s.Guard.Finish()
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
	tmdbID, season, ok, err := s.resolveMapping(ctx, norm, item, mapping, ok)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		res.Errors = append(res.Errors, err)
		return res
	}
	if !ok || tmdbID <= 0 || season <= 0 {
		return res
	}
	addedErr := s.addEpisodeSource(ctx, tmdbID, season, norm)
	if addedErr != nil {
		span.RecordError(addedErr)
		span.SetStatus(codes.Error, addedErr.Error())
		res.Errors = append(res.Errors, addedErr)
		return res
	}
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
) (tmdbID int, season int, ok bool, err error) {
	if matched {
		return override.TMDBID, override.Season, true, nil
	}
	if s.TMDB == nil {
		s.addUnmatched(ctx, Result{}, norm, item)
		return 0, 0, false, nil
	}
	search, found, searchErr := s.TMDB.SearchSeries(ctx, norm.RSSNameKey)
	if searchErr != nil {
		return 0, 0, false, searchErr
	}
	if !found {
		s.addUnmatched(ctx, Result{}, norm, item)
		return 0, 0, false, nil
	}
	return search.TMDBID, 1, true, nil
}

func (s *Service) addEpisodeSource(ctx context.Context, tmdbID, season int, norm NormalizedRelease) error {
	_, addErr := s.Catalog.AddEpisodeSource(ctx, tmdbID, season, norm.Episode, domain.Source{
		Provider:   norm.Provider,
		MagnetLink: norm.MagnetLink,
		Quality:    norm.Quality,
	})
	return addErr
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
			if len(anime.Genres) == 0 && len(details.Genres) > 0 {
				anime.Genres = details.Genres
				needsUpdate = true
			}
			if anime.Rating == 0 && details.Rating > 0 {
				anime.Rating = details.Rating
				needsUpdate = true
			}
			if anime.PosterPath == "" && details.PosterPath != "" {
				anime.PosterPath = details.PosterPath
				needsUpdate = true
			}
		}
	}

	if needsUpdate {
		anime.UpdatedAt = time.Now().UTC()
		return s.Catalog.UpsertSeason(ctx, anime)
	}
	return nil
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
