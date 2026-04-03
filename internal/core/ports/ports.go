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

// CatalogAdmin manages RSS sources and persisted sync status from the DB, and exposes the live catalog
// used by Stremio (memory store), matching the GoTV split: CatalogAdmin for admin/DB + hot cache for public reads.
type CatalogAdmin interface {
	CreateRSSSource(ctx context.Context, url, label string) (*domain.RSSSource, error)
	ListRSSSources(ctx context.Context) ([]domain.RSSSource, error)
	DeleteRSSSource(ctx context.Context, id string) error
	LoadSyncStatus(ctx context.Context) (domain.CatalogSnapshot, error)

	PersistActiveCatalog(ctx context.Context) error

	Snapshot() domain.CatalogSnapshot
	SeriesByID(seriesID string) (domain.CatalogSeries, bool)
	ItemByID(id string) (domain.CatalogItem, bool)
	AniListEnrichment(seriesID string) domain.AniListSeriesEnrichment
	MergeAniListEnrichment(seriesID string, add domain.AniListSeriesEnrichment)
}

// SyncRunner runs RSS fetch + parse + filter.
type SyncRunner interface {
	Run(ctx context.Context) domain.SyncResult
}
