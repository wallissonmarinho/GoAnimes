package app

import (
	"context"
	"os"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/state"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/storage"
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
	mem.Set(snap)
}

// NewRSSSourceAdmin wires admin use case.
func NewRSSSourceAdmin(repo *storage.Catalog) *services.RSSSourceAdminService {
	return services.NewRSSSourceAdminService(repo)
}

// NewRSSSyncService builds sync with concrete deps.
func NewRSSSyncService(repo *storage.Catalog, mem *state.CatalogStore, o services.RSSSyncRuntimeOptions) *services.RSSSyncService {
	return services.NewRSSSyncService(repo, mem, o, nil)
}

// AdminAPIKey returns GOANIMES_ADMIN_API_KEY or ADMIN_API_KEY.
func AdminAPIKey() string {
	if v := os.Getenv("GOANIMES_ADMIN_API_KEY"); v != "" {
		return v
	}
	return os.Getenv("ADMIN_API_KEY")
}
