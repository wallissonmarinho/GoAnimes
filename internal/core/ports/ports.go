package ports

import (
	"context"
	"time"

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
	// ReplaceAniListSynopsis overwrites cached synopsis and the catalog row (e.g. optional machine translation).
	ReplaceAniListSynopsis(seriesID, description string)
	// ReplaceStremioHeroBackground sets the resolved wide backdrop for Stremio (sync or lazy TMDB).
	ReplaceStremioHeroBackground(seriesID, backgroundURL string)
}

// SyncRunner runs RSS fetch + parse + filter.
type SyncRunner interface {
	Run(ctx context.Context) domain.SyncResult
	// SyncRunning is true while Run is executing (interval job or manual rebuild).
	SyncRunning() bool
	// SyncRunStartedAt is UTC start of the in-progress Run, or zero if not running.
	SyncRunStartedAt() time.Time
	// RSSMainFeedsChanged conditional-GETs each configured top-level feed URL (not Erai per-anime feeds).
	// Returns true when a feed is new, its body changed vs the last probe/full sync, or a feed URL was removed from config.
	// Callers should skip while SyncRunning.
	RSSMainFeedsChanged(ctx context.Context) bool
}
