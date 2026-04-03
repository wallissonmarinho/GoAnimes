package state

import (
	"context"
	"sync"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

// CatalogStore holds the latest Stremio catalog in RAM (hydrated from DB at startup and updated on sync).
// saveMu serializes snapshot writes so lazy Stremio enrichment persistence does not race RSS sync saves.
type CatalogStore struct {
	mu     sync.RWMutex
	saveMu sync.Mutex
	snap   domain.CatalogSnapshot
}

// Set replaces the in-memory snapshot.
func (c *CatalogStore) Set(snap domain.CatalogSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snap = snap
}

// Snapshot returns a copy of the current snapshot.
func (c *CatalogStore) Snapshot() domain.CatalogSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snap
}

// ItemByID returns one item by Stremio id.
func (c *CatalogStore) ItemByID(id string) (domain.CatalogItem, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, it := range c.snap.Items {
		if it.ID == id {
			return it, true
		}
	}
	return domain.CatalogItem{}, false
}

// SeriesByID returns catalog row for a series id.
func (c *CatalogStore) SeriesByID(seriesID string) (domain.CatalogSeries, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, s := range c.snap.Series {
		if s.ID == seriesID {
			return s, true
		}
	}
	for _, it := range c.snap.Items {
		if it.SeriesID == seriesID {
			out := domain.CatalogSeries{
				ID:     seriesID,
				Name:   it.SeriesName,
				Poster: domain.SeriesPosterURL(it.SeriesName),
			}
			if en, ok := c.snap.AniListBySeries[seriesID]; ok {
				domain.ApplyEnrichmentToCatalogSeries(&out, en)
			}
			return out, true
		}
	}
	return domain.CatalogSeries{}, false
}

// AniListEnrichment returns cached AniList metadata for a series id (may be empty struct).
func (c *CatalogStore) AniListEnrichment(seriesID string) domain.AniListSeriesEnrichment {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snap.AniListBySeries[seriesID]
}

// MergeAniListEnrichment merges add into the in-memory row for seriesID (e.g. lazy meta fetch).
func (c *CatalogStore) MergeAniListEnrichment(seriesID string, add domain.AniListSeriesEnrichment) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.snap.AniListBySeries == nil {
		c.snap.AniListBySeries = make(map[string]domain.AniListSeriesEnrichment)
	}
	cur := c.snap.AniListBySeries[seriesID]
	merged := domain.MergeAniListEnrichment(cur, add)
	c.snap.AniListBySeries[seriesID] = merged
	for i := range c.snap.Series {
		if c.snap.Series[i].ID == seriesID {
			domain.ApplyEnrichmentToCatalogSeries(&c.snap.Series[i], merged)
			break
		}
	}
}

// ItemsBySeriesID returns episodes for a series, sorted for display.
func (c *CatalogStore) ItemsBySeriesID(seriesID string) []domain.CatalogItem {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return domain.SortEpisodes(c.snap.Items, seriesID)
}

// SetAndPersist replaces the snapshot in memory and writes it to the catalog repository (used by RSS sync).
func (c *CatalogStore) SetAndPersist(ctx context.Context, repo ports.CatalogRepository, snap domain.CatalogSnapshot) error {
	if repo == nil {
		c.Set(snap)
		return nil
	}
	c.saveMu.Lock()
	defer c.saveMu.Unlock()
	c.mu.Lock()
	c.snap = snap
	c.mu.Unlock()
	return repo.SaveCatalogSnapshot(ctx, snap)
}

// PersistSnapshot writes the current in-memory snapshot to the repository (e.g. after lazy AniList/Jikan merge in Stremio meta).
func (c *CatalogStore) PersistSnapshot(ctx context.Context, repo ports.CatalogRepository) error {
	if repo == nil {
		return nil
	}
	c.saveMu.Lock()
	defer c.saveMu.Unlock()
	c.mu.RLock()
	defer c.mu.RUnlock()
	return repo.SaveCatalogSnapshot(ctx, c.snap)
}
