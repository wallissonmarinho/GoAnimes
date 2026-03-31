package state

import (
	"strings"
	"sync"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

// CatalogStore holds the latest Stremio catalog in RAM.
type CatalogStore struct {
	mu   sync.RWMutex
	snap domain.CatalogSnapshot
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
			poster := domain.SeriesPosterURL(it.SeriesName)
			if en, ok := c.snap.AniListBySeries[seriesID]; ok {
				if u := strings.TrimSpace(en.PosterURL); u != "" {
					poster = u
				}
			}
			return domain.CatalogSeries{
				ID:     seriesID,
				Name:   it.SeriesName,
				Poster: poster,
			}, true
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
	c.snap.AniListBySeries[seriesID] = domain.MergeAniListEnrichment(cur, add)
}

// ItemsBySeriesID returns episodes for a series, sorted for display.
func (c *CatalogStore) ItemsBySeriesID(seriesID string) []domain.CatalogItem {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return domain.SortEpisodes(c.snap.Items, seriesID)
}
