package state

import (
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
