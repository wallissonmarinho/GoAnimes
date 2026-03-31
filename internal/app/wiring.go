package app

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/anilist"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/state"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/storage"
	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
	"github.com/wallissonmarinho/GoAnimes/internal/core/services"
)

// OpenCatalog opens the catalog database.
func OpenCatalog(dsn string) (*storage.Catalog, error) {
	return storage.Open(dsn)
}

// HydrateCatalogStore loads the last snapshot from DB into memory.
func HydrateCatalogStore(ctx context.Context, repo ports.CatalogRepository, mem *state.CatalogStore) {
	snap, err := repo.LoadCatalogSnapshot(ctx)
	if err != nil || len(snap.Items) == 0 {
		return
	}
	domain.EnsureSnapshotGrouped(&snap)
	domain.ApplyAniListPostersToSeries(&snap)
	mem.Set(snap)
}

// NewRSSSourceAdmin wires admin use case.
func NewRSSSourceAdmin(repo *storage.Catalog) *services.RSSSourceAdminService {
	return services.NewRSSSourceAdminService(repo)
}

// NewRSSSyncService builds sync with concrete deps.
func NewRSSSyncService(repo *storage.Catalog, mem *state.CatalogStore, o services.RSSSyncRuntimeOptions) *services.RSSSyncService {
	if !anilistDisabled() {
		// Smaller cap for JSON POST bodies; AniList responses are tiny.
		g := httpclient.NewGetter(o.HTTPTimeout, o.UserAgent, 2<<20)
		o.AniList = anilist.NewClient(g)
		if o.AniListMinDelay <= 0 {
			if d, err := time.ParseDuration(getenv("GOANIMES_ANILIST_MIN_DELAY", "750ms")); err == nil {
				o.AniListMinDelay = d
			}
		}
	}
	return services.NewRSSSyncService(repo, mem, o, nil)
}

func anilistDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GOANIMES_ANILIST_DISABLED")))
	return v == "1" || v == "true" || v == "yes"
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// AdminAPIKey returns GOANIMES_ADMIN_API_KEY or ADMIN_API_KEY.
func AdminAPIKey() string {
	if v := os.Getenv("GOANIMES_ADMIN_API_KEY"); v != "" {
		return v
	}
	return os.Getenv("ADMIN_API_KEY")
}
