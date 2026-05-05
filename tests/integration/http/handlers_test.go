package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/http/api"
	"github.com/wallissonmarinho/GoAnimes/internal/app/admin"
	"github.com/wallissonmarinho/GoAnimes/internal/app/stremio"
	syncsvc "github.com/wallissonmarinho/GoAnimes/internal/app/sync"
	"github.com/wallissonmarinho/GoAnimes/internal/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/ports"
)

// mockCatalogRepository implements ports.CatalogRepository for testing
type mockCatalogRepository struct {
	animes []domain.Anime
	genres []string
}

func (m *mockCatalogRepository) UpsertSeason(ctx context.Context, anime domain.Anime) error {
	return nil
}

func (m *mockCatalogRepository) AddEpisodeSource(ctx context.Context, tmdbID, season, episode int, src domain.Source) (bool, error) {
	return true, nil
}

func (m *mockCatalogRepository) GetByTMDBSeason(ctx context.Context, tmdbID, season int) (domain.Anime, bool, error) {
	if len(m.animes) > 0 {
		return m.animes[0], true, nil
	}
	return domain.Anime{}, false, nil
}

func (m *mockCatalogRepository) ListByGenre(ctx context.Context, genre string, limit, skip int) ([]domain.Anime, error) {
	return m.animes, nil
}

func (m *mockCatalogRepository) ListRecent(ctx context.Context, days, limit, skip int) ([]domain.Anime, error) {
	return m.animes, nil
}

func (m *mockCatalogRepository) ListAll(ctx context.Context, limit, skip int) ([]domain.Anime, error) {
	return m.animes, nil
}

func (m *mockCatalogRepository) ListGenres(ctx context.Context) ([]string, error) {
	return m.genres, nil
}

func (m *mockCatalogRepository) RemoveSourcesByProvider(ctx context.Context, provider string) (int, error) {
	return 0, nil
}

// mockFeedRepository implements ports.FeedRepository for testing
type mockFeedRepository struct {
	feeds []*domain.Feed
}

func (m *mockFeedRepository) ListEnabled(ctx context.Context) ([]domain.Feed, error) {
	result := make([]domain.Feed, 0)
	for _, f := range m.feeds {
		if f != nil {
			result = append(result, *f)
		}
	}
	return result, nil
}

func (m *mockFeedRepository) ListAll(ctx context.Context) ([]domain.Feed, error) {
	result := make([]domain.Feed, 0)
	for _, f := range m.feeds {
		if f != nil {
			result = append(result, *f)
		}
	}
	return result, nil
}

func (m *mockFeedRepository) Upsert(ctx context.Context, feed domain.Feed) (domain.Feed, error) {
	return feed, nil
}

func (m *mockFeedRepository) Delete(ctx context.Context, id string) error {
	return nil
}

// mockMappingRepository implements ports.MappingRepository for testing
type mockMappingRepository struct {
	overrides []domain.MappingOverride
}

func (m *mockMappingRepository) FindOverride(ctx context.Context, rssNameKey string) (domain.MappingOverride, bool, error) {
	return domain.MappingOverride{}, false, nil
}

func (m *mockMappingRepository) UpsertOverride(ctx context.Context, override domain.MappingOverride) (domain.MappingOverride, error) {
	return override, nil
}

func (m *mockMappingRepository) ListOverrides(ctx context.Context) ([]domain.MappingOverride, error) {
	return m.overrides, nil
}

func (m *mockMappingRepository) AddUnmatched(ctx context.Context, release domain.UnmatchedRelease) error {
	return nil
}

func (m *mockMappingRepository) ListUnmatched(ctx context.Context, limit int) ([]domain.UnmatchedRelease, error) {
	return nil, nil
}

// mockFeedReader implements ports.FeedReader for testing
type mockFeedReader struct{}

func (m *mockFeedReader) Fetch(ctx context.Context, feed domain.Feed) ([]ports.ReleaseItem, error) {
	return []ports.ReleaseItem{}, nil
}

// mockTMDBClient implements ports.TMDBClient for testing
type mockTMDBClient struct{}

func (m *mockTMDBClient) SearchSeries(ctx context.Context, query string) (ports.TMDBSearchResult, bool, error) {
	return ports.TMDBSearchResult{}, false, nil
}

func (m *mockTMDBClient) GetSeasonDetails(ctx context.Context, tmdbID, season int) (ports.TMDBSeasonDetails, error) {
	return ports.TMDBSeasonDetails{}, nil
}

var _ ports.CatalogRepository = (*mockCatalogRepository)(nil)
var _ ports.FeedRepository = (*mockFeedRepository)(nil)
var _ ports.MappingRepository = (*mockMappingRepository)(nil)
var _ ports.FeedReader = (*mockFeedReader)(nil)
var _ ports.TMDBClient = (*mockTMDBClient)(nil)

func TestHealthEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	deps := api.Deps{
		Stremio:  &stremio.Service{Repo: &mockCatalogRepository{}},
		Sync:     &syncsvc.Service{},
		Admin:    &admin.Service{Feeds: &mockFeedRepository{}, Mapping: &mockMappingRepository{}, Catalog: &mockCatalogRepository{}},
		AdminKey: "test-key",
	}

	api.Register(engine, deps)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestManifestEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	deps := api.Deps{
		Stremio:  &stremio.Service{Repo: &mockCatalogRepository{}},
		Sync:     &syncsvc.Service{},
		Admin:    &admin.Service{Feeds: &mockFeedRepository{}, Mapping: &mockMappingRepository{}, Catalog: &mockCatalogRepository{}},
		AdminKey: "test-key",
	}

	api.Register(engine, deps)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/manifest.json", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var manifest map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &manifest)
	require.NoError(t, err)
	require.NotNil(t, manifest)
}

func TestCatalogEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	anime := domain.Anime{
		TMDBID:       123,
		SeasonNumber: 1,
		Title:        "Test Anime",
	}
	catalogRepo := &mockCatalogRepository{
		animes: []domain.Anime{anime},
	}

	deps := api.Deps{
		Stremio:  &stremio.Service{Repo: catalogRepo},
		Sync:     &syncsvc.Service{},
		Admin:    &admin.Service{Feeds: &mockFeedRepository{}, Mapping: &mockMappingRepository{}, Catalog: &mockCatalogRepository{}},
		AdminKey: "test-key",
	}

	api.Register(engine, deps)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/catalog/anime/goanimes.json", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "metas")
}

func TestSyncEndpoint_Accepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	deps := api.Deps{
		Stremio: &stremio.Service{Repo: &mockCatalogRepository{}},
		Sync: &syncsvc.Service{
			Feeds:   &mockFeedRepository{},
			Mapping: &mockMappingRepository{},
			Catalog: &mockCatalogRepository{},
			Reader:  &mockFeedReader{},
			TMDB:    &mockTMDBClient{},
		},
		Admin:    &admin.Service{Feeds: &mockFeedRepository{}, Mapping: &mockMappingRepository{}, Catalog: &mockCatalogRepository{}},
		AdminKey: "test-key",
	}

	api.Register(engine, deps)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/sync?force=true", nil)
	req.Header.Set("X-Admin-Key", "test-key")
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	require.True(t, response["accepted"].(bool))
	require.Equal(t, "force sync scheduled", response["message"])
}
