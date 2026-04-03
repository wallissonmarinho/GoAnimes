package app

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/anilist"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/jikan"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/translate"
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
	domain.ApplyAniListEnrichmentToSeries(&snap)
	mem.Set(snap)
}

// NewCatalogAdmin wires admin + Stremio catalog façade (repo + in-memory store).
func NewCatalogAdmin(repo *storage.Catalog, store *state.CatalogStore) *services.CatalogAdminService {
	return services.NewCatalogAdminService(repo, store)
}

// SynopsisTranslatorFromEnv builds optional synopsis translation from translate.FromEnv (gilang) when
// GOANIMES_GOOGLE_GTX_TRANSLATE or GOANIMES_GOOGLE_CLIENTS5_TRANSLATE is set. Uses same HTTP timeout as RSS sync getter.
func SynopsisTranslatorFromEnv(httpTimeout time.Duration, userAgent string, maxBody int64) ports.SynopsisTranslator {
	if httpTimeout <= 0 {
		httpTimeout = 45 * time.Second
	}
	if maxBody <= 0 {
		maxBody = 50 << 20
	}
	g := httpclient.NewGetter(httpTimeout, userAgent, maxBody)
	return translate.FromEnv(g)
}

// NewRSSSyncService builds sync with concrete deps and returns optional API clients for HTTP handlers.
func NewRSSSyncService(repo *storage.Catalog, mem *state.CatalogStore, o services.RSSSyncRuntimeOptions) (*services.RSSSyncService, *anilist.Client, *jikan.Client) {
	var al *anilist.Client
	if !anilistDisabled() {
		// Smaller cap for JSON POST bodies; AniList responses are tiny.
		g := httpclient.NewGetter(o.HTTPTimeout, o.UserAgent, 2<<20)
		al = anilist.NewClient(g)
		o.AniList = al
		if o.AniListMinDelay <= 0 {
			if d, err := time.ParseDuration(getenv("GOANIMES_ANILIST_MIN_DELAY", "0")); err == nil {
				o.AniListMinDelay = d
			}
		}
	}
	var jk *jikan.Client
	if !jikanDisabled() {
		g := httpclient.NewGetter(o.HTTPTimeout, o.UserAgent, 2<<20)
		jk = jikan.NewClient(g)
		o.Jikan = jk
		if o.JikanMinDelay <= 0 {
			if d, err := time.ParseDuration(getenv("GOANIMES_JIKAN_MIN_DELAY", "900ms")); err == nil {
				o.JikanMinDelay = d
			}
		}
	}
	return services.NewRSSSyncService(repo, mem, o, nil), al, jk
}

func anilistDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GOANIMES_ANILIST_DISABLED")))
	return v == "1" || v == "true" || v == "yes"
}

func jikanDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GOANIMES_JIKAN_DISABLED")))
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
