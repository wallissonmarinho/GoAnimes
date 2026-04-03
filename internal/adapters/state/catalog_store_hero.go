package state

import (
	"strings"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

// ReplaceStremioHeroBackground sets the cached Stremio meta.background URL (sync or lazy TMDB pick).
func (c *CatalogStore) ReplaceStremioHeroBackground(seriesID, backgroundURL string) {
	backgroundURL = strings.TrimSpace(backgroundURL)
	if seriesID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.snap.AniListBySeries == nil {
		c.snap.AniListBySeries = make(map[string]domain.AniListSeriesEnrichment)
	}
	e := c.snap.AniListBySeries[seriesID]
	e.StremioHeroBackgroundURL = backgroundURL
	c.snap.AniListBySeries[seriesID] = e
}
