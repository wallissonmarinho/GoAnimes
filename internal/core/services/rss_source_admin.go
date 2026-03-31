package services

import (
	"context"
	"strings"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

// RSSSourceAdminService implements ports.RSSAdmin.
type RSSSourceAdminService struct {
	repo ports.CatalogRepository
}

func NewRSSSourceAdminService(repo ports.CatalogRepository) *RSSSourceAdminService {
	return &RSSSourceAdminService{repo: repo}
}

func (s *RSSSourceAdminService) CreateRSSSource(ctx context.Context, url, label string) (*domain.RSSSource, error) {
	u := strings.TrimSpace(url)
	if u == "" || (!strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://")) {
		return nil, domain.ErrInvalidSourceURL
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

func (s *RSSSourceAdminService) ListRSSSources(ctx context.Context) ([]domain.RSSSource, error) {
	return s.repo.ListRSSSources(ctx)
}

func (s *RSSSourceAdminService) DeleteRSSSource(ctx context.Context, id string) error {
	return s.repo.DeleteRSSSource(ctx, id)
}

func (s *RSSSourceAdminService) LoadSyncStatus(ctx context.Context) (domain.CatalogSnapshot, error) {
	return s.repo.LoadCatalogSnapshot(ctx)
}
