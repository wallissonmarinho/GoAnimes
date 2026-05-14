package sync_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/app/sync"
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

func (f *fakeMappingRepo) DeleteOverride(ctx context.Context, id string) error {
	return nil
}

func (f *fakeMappingRepo) DeleteUnmatched(ctx context.Context, id string) error {
	return nil
}

type fakeCatalogRepo struct {
	items   map[string]domain.Anime
	listAll []domain.Anime
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

func (f *fakeCatalogRepo) UpdateEpisodeDetails(ctx context.Context, tmdbID, season, episode int, title, overview, stillPath string) error {
	if f.items == nil {
		return nil
	}
	key := catalogKey(tmdbID, season)
	anime, ok := f.items[key]
	if !ok {
		return nil
	}
	for i := range anime.Episodes {
		if anime.Episodes[i].Number == episode {
			anime.Episodes[i].Title = title
			anime.Episodes[i].Overview = overview
			anime.Episodes[i].StillPath = stillPath
			f.items[key] = anime
			break
		}
	}
	return nil
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
	return f.listAll, nil
}

func (f *fakeCatalogRepo) ListGenres(ctx context.Context) ([]string, error) {
	return []string{}, nil
}

func (f *fakeCatalogRepo) RemoveSourcesByProvider(ctx context.Context, provider string) (int, error) {
	return 0, nil
}

type fakeFeedReader struct {
	items []ports.ReleaseItem
	err   error
}

func (f *fakeFeedReader) Fetch(ctx context.Context, feed domain.Feed) ([]ports.ReleaseItem, error) {
	return f.items, f.err
}

type fakeTMDBClient struct {
	searchResult      ports.TMDBSearchResult
	found             bool
	searchErr         error
	details           ports.TMDBSeasonDetails
	detailsErr        error
	episodeDetails    ports.TMDBEpisodeDetails
	episodeDetailsErr error
}

func (f *fakeTMDBClient) SearchSeries(ctx context.Context, query string) (ports.TMDBSearchResult, bool, error) {
	return f.searchResult, f.found, f.searchErr
}

func (f *fakeTMDBClient) GetSeasonDetails(ctx context.Context, tmdbID, season int) (ports.TMDBSeasonDetails, error) {
	return f.details, f.detailsErr
}

func (f *fakeTMDBClient) GetEpisodeDetails(ctx context.Context, tmdbID, season, episode int) (ports.TMDBEpisodeDetails, error) {
	return f.episodeDetails, f.episodeDetailsErr
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
	service := &sync.Service{
		Feeds:   &fakeFeedRepo{feeds: feeds},
		Mapping: mapping,
		Catalog: catalog,
		Reader:  reader,
		TMDB:    nil,
		Guard:   &sync.Guard{},
	}

	res := service.Run(context.Background())
	if len(res.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", res.Errors)
	}
	if res.Processed != 1 {
		t.Fatalf("expected processed 1, got %d (unmatched=%d catalog=%d)", res.Processed, len(mapping.unmatched), len(catalog.items))
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
	service := &sync.Service{
		Feeds:   &fakeFeedRepo{feeds: []domain.Feed{{ID: "f1", Name: "Erai", URL: "http://example", Type: domain.FeedTypeRSS, Enabled: true}}},
		Mapping: mapping,
		Catalog: &fakeCatalogRepo{},
		Reader:  reader,
		TMDB:    nil,
		Guard:   &sync.Guard{},
	}

	res := service.Run(context.Background())
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", res.Errors)
	}
	if len(mapping.unmatched) != 1 {
		t.Fatalf("expected 1 unmatched, got %d", len(mapping.unmatched))
	}
}

func TestSyncRunAddsUnmatchedWhenAutomaticTMDBMatchLooksIncoherent(t *testing.T) {
	reader := &fakeFeedReader{items: []ports.ReleaseItem{{
		Title:     "[Erai] Kanojo Okarishimasu 2nd Season - 03",
		Link:      "magnet:?xt=urn:btih:def",
		Provider:  "Erai",
		Published: time.Now(),
	}}}
	mapping := &fakeMappingRepo{overrides: map[string]domain.MappingOverride{}}
	service := &sync.Service{
		Feeds:   &fakeFeedRepo{feeds: []domain.Feed{{ID: "f1", Name: "Erai", URL: "http://example", Type: domain.FeedTypeRSS, Enabled: true}}},
		Mapping: mapping,
		Catalog: &fakeCatalogRepo{},
		Reader:  reader,
		TMDB: &fakeTMDBClient{
			searchResult: ports.TMDBSearchResult{TMDBID: 196950, Title: "Witch Hat Atelier"},
			found:        true,
			details: ports.TMDBSeasonDetails{
				Title:         "Witch Hat Atelier",
				OriginalTitle: "とんがり帽子のアトリエ",
			},
		},
		Guard: &sync.Guard{},
	}

	res := service.Run(context.Background())
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", res.Errors)
	}
	if res.Processed != 0 {
		t.Fatalf("expected processed 0, got %d", res.Processed)
	}
	if len(mapping.unmatched) != 1 {
		t.Fatalf("expected 1 unmatched, got %d", len(mapping.unmatched))
	}
}

func TestSyncRunAddsUnmatchedWhenAutomaticTMDBMatchIgnoresExplicitSeasonHint(t *testing.T) {
	reader := &fakeFeedReader{items: []ports.ReleaseItem{{
		Title:     "[ToonsHub] Mission Yozakura Family S02E02 1080p DSNP WEB-DL AAC2.0 H.264",
		Link:      "magnet:?xt=urn:btih:yozakura-s02e02",
		Provider:  "nekobt ToonsHub",
		Published: time.Now(),
	}}}
	mapping := &fakeMappingRepo{overrides: map[string]domain.MappingOverride{}}
	catalog := &fakeCatalogRepo{}
	service := &sync.Service{
		Feeds:   &fakeFeedRepo{feeds: []domain.Feed{{ID: "f1", Name: "nekobt ToonsHub", URL: "http://example", Type: domain.FeedTypeRSS, Enabled: true}}},
		Mapping: mapping,
		Catalog: catalog,
		Reader:  reader,
		TMDB: &fakeTMDBClient{
			searchResult: ports.TMDBSearchResult{TMDBID: 216467, Title: "Mission: Yozakura Family"},
			found:        true,
			details: ports.TMDBSeasonDetails{
				Title:         "Mission: Yozakura Family",
				OriginalTitle: "Mission: Yozakura Family",
			},
		},
		Guard: &sync.Guard{},
	}

	res := service.Run(context.Background())
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", res.Errors)
	}
	if res.Processed != 0 {
		t.Fatalf("expected processed 0, got %d", res.Processed)
	}
	if len(mapping.unmatched) != 1 {
		t.Fatalf("expected 1 unmatched, got %d", len(mapping.unmatched))
	}
	if len(catalog.items) != 0 {
		t.Fatalf("expected no catalog writes, got %d", len(catalog.items))
	}
}

func TestSyncRunMapsOnePieceEpisodesWithoutManualOverride(t *testing.T) {
	feeds := []domain.Feed{{ID: "f1", Name: "Erai One Piece", URL: "http://example", Type: domain.FeedTypeRSS, Enabled: true}}
	reader := &fakeFeedReader{items: []ports.ReleaseItem{{
		Title:     "[Magnet] One Piece - 1105 (Multi) [SD][us][br][mx][es][sa][fr][de][it][ru][Airing]",
		Link:      "magnet:?xt=urn:btih:onepiece1105",
		Provider:  "Erai One Piece",
		Published: time.Now(),
	}}}
	mapping := &fakeMappingRepo{overrides: map[string]domain.MappingOverride{}}
	catalog := &fakeCatalogRepo{}
	service := &sync.Service{
		Feeds:   &fakeFeedRepo{feeds: feeds},
		Mapping: mapping,
		Catalog: catalog,
		Reader:  reader,
		TMDB:    nil,
		Guard:   &sync.Guard{},
	}

	key, ep, _ := sync.NormalizeTitle("[Magnet] One Piece - 1105 (Multi) [SD][us][br][mx][es][sa][fr][de][it][ru][Airing]")
	if key != "one piece - 1105" || ep != 0 {
		t.Fatalf("unexpected normalization: key=%q ep=%d", key, ep)
	}

	res := service.Run(context.Background())
	if len(res.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", res.Errors)
	}
	if res.Processed != 1 {
		t.Fatalf("expected processed 1, got %d (unmatched=%d catalog=%d)", res.Processed, len(mapping.unmatched), len(catalog.items))
	}
	anime, found, _ := catalog.GetByTMDBSeason(context.Background(), 37854, 1)
		if !found {
			t.Fatalf("expected one piece to be mapped by generic override")
		}
	foundEpisode := false
	for _, ep := range anime.Episodes {
		if ep.Number == 1105 {
			foundEpisode = true
			break
		}
	}
	if !foundEpisode {
		t.Fatalf("expected episode 1105 to be added")
	}
	if len(mapping.unmatched) != 0 {
		t.Fatalf("expected no unmatched entries, got %d", len(mapping.unmatched))
	}
}

func TestSyncRunIgnoresOnePieceSpecialsAndBatches(t *testing.T) {
	feeds := []domain.Feed{{ID: "f1", Name: "Erai One Piece", URL: "http://example", Type: domain.FeedTypeRSS, Enabled: true}}
	reader := &fakeFeedReader{items: []ports.ReleaseItem{
		{
			Title:     "[Magnet] One Piece - 3D2Y [SD][us][br][mx][Movie or Special Episode]",
			Link:      "magnet:?xt=urn:btih:onepiece3d2y",
			Provider:  "Erai One Piece",
			Published: time.Now(),
		},
		{
			Title:     "[Magnet] One Piece - 0892 ~ 1089 [SD][us][br][mx][es][sa][fr][de][it][ru][Batch]",
			Link:      "magnet:?xt=urn:btih:onepiecebatch",
			Provider:  "Erai One Piece",
			Published: time.Now(),
		},
	}}
	mapping := &fakeMappingRepo{overrides: map[string]domain.MappingOverride{}}
	catalog := &fakeCatalogRepo{}
	service := &sync.Service{
		Feeds:   &fakeFeedRepo{feeds: feeds},
		Mapping: mapping,
		Catalog: catalog,
		Reader:  reader,
		TMDB:    nil,
		Guard:   &sync.Guard{},
	}

	res := service.Run(context.Background())
	if len(res.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", res.Errors)
	}
	if res.Processed != 0 {
		t.Fatalf("expected processed 0, got %d", res.Processed)
	}
	if len(mapping.unmatched) != 0 {
		t.Fatalf("expected no unmatched entries, got %d", len(mapping.unmatched))
	}
}

func TestSyncRunReportsFetchError(t *testing.T) {
	reader := &fakeFeedReader{err: errors.New("fetch failed")}
	service := &sync.Service{
		Feeds:   &fakeFeedRepo{feeds: []domain.Feed{{ID: "f1", Name: "Erai", URL: "http://example", Type: domain.FeedTypeRSS, Enabled: true}}},
		Mapping: &fakeMappingRepo{},
		Catalog: &fakeCatalogRepo{},
		Reader:  reader,
		TMDB:    &fakeTMDBClient{},
		Guard:   &sync.Guard{},
	}

	res := service.Run(context.Background())
	if len(res.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(res.Errors))
	}
}

func TestSyncForceBackfillsMissingAnimeDetails(t *testing.T) {
	catalog := &fakeCatalogRepo{
		listAll: []domain.Anime{{
			TMDBID:       300126,
			SeasonNumber: 1,
			Title:        "Liar Game",
		}},
		items: map[string]domain.Anime{
			catalogKey(300126, 1): {
				TMDBID:       300126,
				SeasonNumber: 1,
				Title:        "Old Title",
				Status:       "ended",
				LastEpisodeAt:"2026-04-01",
			},
		},
	}
	service := &sync.Service{
		Feeds:   &fakeFeedRepo{feeds: []domain.Feed{}},
		Mapping: &fakeMappingRepo{},
		Catalog: catalog,
		Reader:  &fakeFeedReader{},
		TMDB: &fakeTMDBClient{details: ports.TMDBSeasonDetails{
			Title:             "Liar Game",
			Overview:          "Sinopse preenchida",
			PosterPath:        "/poster.jpg",
			BackdropPath:      "/backdrop.jpg",
			Genres:            []string{"Drama"},
			Rating:            8.7,
			Status:            "Returning Series",
			InProduction:      true,
			HasNextEpisode:    true,
			LastEpisodeAirDate:"2026-05-05",
			LastEpisodeNumber: 4,
			NextEpisodeAirDate:"2026-05-12",
			NextEpisodeNumber: 5,
		}},
		Guard: &sync.Guard{},
	}

	res := service.Run(context.Background())
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", res.Errors)
	}
	anime, found, _ := catalog.GetByTMDBSeason(context.Background(), 300126, 1)
	if !found {
		t.Fatal("expected anime to be present")
	}
	if anime.Overview != "" {
		t.Fatal("expected normal sync to keep missing details untouched")
	}
	if anime.PosterPath != "" || anime.BackdropPath != "" {
		t.Fatal("expected normal sync to keep missing details untouched")
	}

	forceRes := service.ForceRun(context.Background())
	if len(forceRes.Errors) != 0 {
		t.Fatalf("unexpected force sync errors: %v", forceRes.Errors)
	}
	anime, found, _ = catalog.GetByTMDBSeason(context.Background(), 300126, 1)
	if !found {
		t.Fatal("expected anime to remain present after force sync")
	}
	if anime.Overview != "Sinopse preenchida" {
		t.Fatalf("expected overview to be backfilled, got %q", anime.Overview)
	}
	if anime.PosterPath != "/poster.jpg" || anime.BackdropPath != "/backdrop.jpg" {
		t.Fatal("expected poster and backdrop to be backfilled")
	}
	if anime.Title != "Liar Game" {
		t.Fatalf("expected title to be refreshed, got %q", anime.Title)
	}
	if anime.Status != "current" {
		t.Fatalf("expected status to be refreshed, got %q", anime.Status)
	}
	if anime.LastEpisodeAt != "2026-05-05" || anime.LastEpisodeNo != 4 {
		t.Fatalf("expected last episode metadata refresh, got at=%q no=%d", anime.LastEpisodeAt, anime.LastEpisodeNo)
	}
	if anime.NextEpisodeAt != "2026-05-12" || anime.NextEpisodeNo != 5 {
		t.Fatalf("expected next episode metadata refresh, got at=%q no=%d", anime.NextEpisodeAt, anime.NextEpisodeNo)
	}
}

func catalogKey(tmdbID, season int) string {
	return fmt.Sprintf("%d:%d", tmdbID, season)
}
