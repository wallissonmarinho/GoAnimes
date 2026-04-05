package ports

import (
	"context"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

// CatalogRepository persists RSS sources and the last catalog snapshot.
type CatalogRepository interface {
	FindRSSSourceByURL(ctx context.Context, url string) (*domain.RSSSource, error)
	CreateRSSSource(ctx context.Context, url, label string) (*domain.RSSSource, error)
	ListRSSSources(ctx context.Context) ([]domain.RSSSource, error)
	DeleteRSSSource(ctx context.Context, id string) error

	SaveCatalogSnapshot(ctx context.Context, snap domain.CatalogSnapshot) error
	LoadCatalogSnapshot(ctx context.Context) (domain.CatalogSnapshot, error)
}
