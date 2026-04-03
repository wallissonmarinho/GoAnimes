package services

import (
	"context"
	"errors"
	"strings"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/state"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

// CatalogAdminService implements ports.CatalogAdmin: RSS CRUD and sync status from DB,
// plus the live catalog view used by Stremio (memory store hydrated from DB, like GoTV’s CatalogAdmin + MemoryStore split).
type CatalogAdminService struct {
	repo  ports.CatalogRepository
	store *state.CatalogStore
}

// NewCatalogAdminService wires admin HTTP and Stremio read paths. repo may be nil in tests (DB-backed ops no-op or empty).
func NewCatalogAdminService(repo ports.CatalogRepository, store *state.CatalogStore) *CatalogAdminService {
	if store == nil {
		store = &state.CatalogStore{}
	}
	return &CatalogAdminService{repo: repo, store: store}
}

// errNoCatalogRepo is returned when HTTP expects a DB but wiring passed a nil repository (e.g. misconfiguration).
var errNoCatalogRepo = errors.New("catalog repository not configured")

func (s *CatalogAdminService) CreateRSSSource(ctx context.Context, url, label string) (*domain.RSSSource, error) {
	u := strings.TrimSpace(url)
	if u == "" || (!strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://")) {
		return nil, domain.ErrInvalidSourceURL
	}
	if s.repo == nil {
		return nil, errNoCatalogRepo
	}
	dup, err := s.repo.FindRSSSourceByURL(ctx, u)
	if err != nil {
		return nil, err
	}
	if dup != nil {
		return nil, domain.ErrDuplicateRSSSourceURL
	}
	return s.repo.CreateRSSSource(ctx, u, strings.TrimSpace(label))
}

func (s *CatalogAdminService) ListRSSSources(ctx context.Context) ([]domain.RSSSource, error) {
	if s.repo == nil {
		return nil, nil
	}
	return s.repo.ListRSSSources(ctx)
}

func (s *CatalogAdminService) DeleteRSSSource(ctx context.Context, id string) error {
	if s.repo == nil {
		return errNoCatalogRepo
	}
	return s.repo.DeleteRSSSource(ctx, id)
}

func (s *CatalogAdminService) LoadSyncStatus(ctx context.Context) (domain.CatalogSnapshot, error) {
	if s.repo == nil {
		return domain.CatalogSnapshot{}, nil
	}
	return s.repo.LoadCatalogSnapshot(ctx)
}

func (s *CatalogAdminService) PersistActiveCatalog(ctx context.Context) error {
	return s.store.PersistSnapshot(ctx, s.repo)
}

func (s *CatalogAdminService) Snapshot() domain.CatalogSnapshot {
	return s.store.Snapshot()
}

func (s *CatalogAdminService) SeriesByID(seriesID string) (domain.CatalogSeries, bool) {
	return s.store.SeriesByID(seriesID)
}

func (s *CatalogAdminService) ItemByID(id string) (domain.CatalogItem, bool) {
	return s.store.ItemByID(id)
}

func (s *CatalogAdminService) AniListEnrichment(seriesID string) domain.AniListSeriesEnrichment {
	return s.store.AniListEnrichment(seriesID)
}

func (s *CatalogAdminService) MergeAniListEnrichment(seriesID string, add domain.AniListSeriesEnrichment) {
	s.store.MergeAniListEnrichment(seriesID, add)
}

var _ ports.CatalogAdmin = (*CatalogAdminService)(nil)
