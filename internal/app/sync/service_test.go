package sync

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/ports"
)

type fakeFeedRepo struct {
	feeds []domain.Feed
	err   error
}

func (f *fakeFeedRepo) ListEnabled(ctx context.Context) ([]domain.Feed, error) {
	return f.feeds, f.err
}

func (f *fakeFeedRepo) ListAll(ctx context.Context) ([]domain.Feed, error) {
	return f.feeds, f.err
}

func (f *fakeFeedRepo) Upsert(ctx context.Context, feed domain.Feed) (domain.Feed, error) {
	return feed, nil
}

func (f *fakeFeedRepo) Delete(ctx context.Context, id string) error {
	return nil
}

type fakeMappingRepo struct {
	overrides map[string]domain.MappingOverride
	unmatched []domain.UnmatchedRelease
}

func (f *fakeMappingRepo) FindOverride(ctx context.Context, rssNameKey string) (domain.MappingOverride, bool, error) {
	override, ok := f.overrides[rssNameKey]
	return override, ok, nil
}

func (f *fakeMappingRepo) UpsertOverride(ctx context.Context, override domain.MappingOverride) (domain.MappingOverride, error) {
	if f.overrides == nil {
		f.overrides = map[string]domain.MappingOverride{}
	}
	f.overrides[override.RSSNameKey] = override
	return override, nil
}

func (f *fakeMappingRepo) ListOverrides(ctx context.Context) ([]domain.MappingOverride, error) {
	out := make([]domain.MappingOverride, 0, len(f.overrides))
	for _, v := range f.overrides {
		out = append(out, v)
	}
	return out, nil
}

func (f *fakeMappingRepo) AddUnmatched(ctx context.Context, release domain.UnmatchedRelease) error {
	f.unmatched = append(f.unmatched, release)
	return nil
}

func (f *fakeMappingRepo) ListUnmatched(ctx context.Context, limit int) ([]domain.UnmatchedRelease, error) {
	return f.unmatched, nil
}

type fakeCatalogRepo struct {
	items map[string]domain.Anime
}

func (f *fakeCatalogRepo) UpsertSeason(ctx context.Context, anime domain.Anime) error {
	if f.items == nil {
		f.items = map[string]domain.Anime{}
	}
	key := catalogKey(anime.TMDBID, anime.SeasonNumber)
	if existing, ok := f.items[key]; ok && len(anime.Episodes) == 0 {
		anime.Episodes = existing.Episodes
	}
	f.items[key] = anime
	return nil
}

func (f *fakeCatalogRepo) AddEpisodeSource(ctx context.Context, tmdbID, season, episode int, src domain.Source) (bool, error) {
	if f.items == nil {
		f.items = map[string]domain.Anime{}
	}
	key := catalogKey(tmdbID, season)
	anime, ok := f.items[key]
	if !ok {
		anime = domain.Anime{TMDBID: tmdbID, SeasonNumber: season}
	}
	ep := anime.EnsureEpisode(episode)
	added := ep.AddSource(src)
	f.items[key] = anime
	return added, nil
}

func (f *fakeCatalogRepo) GetByTMDBSeason(ctx context.Context, tmdbID, season int) (domain.Anime, bool, error) {
	if f.items == nil {
		return domain.Anime{}, false, nil
	}
	anime, ok := f.items[catalogKey(tmdbID, season)]
	return anime, ok, nil
}

func (f *fakeCatalogRepo) ListByGenre(ctx context.Context, genre string, limit, skip int) ([]domain.Anime, error) {
	return nil, nil
}

func (f *fakeCatalogRepo) ListRecent(ctx context.Context, days, limit, skip int) ([]domain.Anime, error) {
	return nil, nil
}

func (f *fakeCatalogRepo) ListAll(ctx context.Context, limit, skip int) ([]domain.Anime, error) {
	return nil, nil
}

func (f *fakeCatalogRepo) ListGenres(ctx context.Context) ([]string, error) {
	return []string{}, nil
}

type fakeFeedReader struct {
	items []ports.ReleaseItem
	err   error
}

func (f *fakeFeedReader) Fetch(ctx context.Context, feed domain.Feed) ([]ports.ReleaseItem, error) {
	return f.items, f.err
}

type fakeTMDBClient struct {
	searchResult ports.TMDBSearchResult
	found        bool
	searchErr    error
	details      ports.TMDBSeasonDetails
	detailsErr   error
}

func (f *fakeTMDBClient) SearchSeries(ctx context.Context, query string) (ports.TMDBSearchResult, bool, error) {
	return f.searchResult, f.found, f.searchErr
}

func (f *fakeTMDBClient) GetSeasonDetails(ctx context.Context, tmdbID, season int) (ports.TMDBSeasonDetails, error) {
	return f.details, f.detailsErr
}

func TestSyncRunWithOverride(t *testing.T) {
	feeds := []domain.Feed{{ID: "f1", Name: "Erai", URL: "http://example", Type: domain.FeedTypeRSS, Enabled: true}}
	reader := &fakeFeedReader{items: []ports.ReleaseItem{{
		Title:     "[Erai] Honzuki no Gekokujou - 03 [br]",
		Link:      "magnet:?xt=urn:btih:abc",
		Provider:  "Erai",
		Published: time.Now(),
	}}}
	mapping := &fakeMappingRepo{overrides: map[string]domain.MappingOverride{
		"honzuki no gekokujou": {TMDBID: 91768, Season: 4},
	}}
	catalog := &fakeCatalogRepo{}
	service := &Service{
		Feeds:   &fakeFeedRepo{feeds: feeds},
		Mapping: mapping,
		Catalog: catalog,
		Reader:  reader,
		TMDB:    nil,
		Guard:   &Guard{},
	}

	res := service.Run(context.Background())
	if len(res.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", res.Errors)
	}
	if res.Processed != 1 {
		t.Fatalf("expected processed 1, got %d", res.Processed)
	}
	anime, found, _ := catalog.GetByTMDBSeason(context.Background(), 91768, 4)
	if !found {
		t.Fatal("expected season to be upserted")
	}
	if len(anime.Episodes) != 1 || len(anime.Episodes[0].Sources) != 1 {
		t.Fatal("expected episode sources to be added")
	}
	if len(mapping.unmatched) != 0 {
		t.Fatalf("expected no unmatched, got %d", len(mapping.unmatched))
	}
}

func TestSyncRunAddsUnmatchedWhenNoTMDB(t *testing.T) {
	reader := &fakeFeedReader{items: []ports.ReleaseItem{{
		Title:     "[Erai] Unknown Anime - 01",
		Link:      "magnet:?xt=urn:btih:def",
		Provider:  "Erai",
		Published: time.Now(),
	}}}
	mapping := &fakeMappingRepo{overrides: map[string]domain.MappingOverride{}}
	service := &Service{
		Feeds:   &fakeFeedRepo{feeds: []domain.Feed{{ID: "f1", Name: "Erai", URL: "http://example", Type: domain.FeedTypeRSS, Enabled: true}}},
		Mapping: mapping,
		Catalog: &fakeCatalogRepo{},
		Reader:  reader,
		TMDB:    nil,
		Guard:   &Guard{},
	}

	res := service.Run(context.Background())
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", res.Errors)
	}
	if len(mapping.unmatched) != 1 {
		t.Fatalf("expected 1 unmatched, got %d", len(mapping.unmatched))
	}
}

func TestSyncRunReportsFetchError(t *testing.T) {
	reader := &fakeFeedReader{err: errors.New("fetch failed")}
	service := &Service{
		Feeds:   &fakeFeedRepo{feeds: []domain.Feed{{ID: "f1", Name: "Erai", URL: "http://example", Type: domain.FeedTypeRSS, Enabled: true}}},
		Mapping: &fakeMappingRepo{},
		Catalog: &fakeCatalogRepo{},
		Reader:  reader,
		TMDB:    &fakeTMDBClient{},
		Guard:   &Guard{},
	}

	res := service.Run(context.Background())
	if len(res.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(res.Errors))
	}
}

func catalogKey(tmdbID, season int) string {
	return fmt.Sprintf("%d:%d", tmdbID, season)
}
