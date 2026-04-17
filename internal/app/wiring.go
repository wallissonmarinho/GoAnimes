package app

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/cinemeta"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/httpclient"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/state"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/storage"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/thetvdb"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/tmdb"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/translate"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
	"github.com/wallissonmarinho/GoAnimes/internal/core/rsssync"
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

// NewCatalogAdmin wires admin + Stremio catalog façade (repo + in-memory store).
func NewCatalogAdmin(repo *storage.Catalog, store *state.CatalogStore) *services.CatalogAdminService {
	return services.NewCatalogAdminService(repo, store)
}

// NewGoaiAuditAdmin wires GoAI audit admin service (HTTP handlers use this, not repo directly).
func NewGoaiAuditAdmin(repo ports.GoAIAuditRepository) *services.GoaiAuditAdminService {
	return services.NewGoaiAuditAdminService(repo)
}

// NewSynopsisTranslator wires gilang Google Translate for AniList synopsis en→pt (always on; no env toggle).
func NewSynopsisTranslator(httpTimeout time.Duration, userAgent string, maxBody int64) ports.SynopsisTranslator {
	if httpTimeout <= 0 {
		httpTimeout = 45 * time.Second
	}
	if maxBody <= 0 {
		maxBody = 50 << 20
	}
	g := httpclient.NewGetter(httpTimeout, userAgent, maxBody)
	return translate.NewSynopsisTranslator(g)
}

// NewRSSSyncService builds sync with concrete deps and returns optional API clients for HTTP handlers.
func NewRSSSyncService(repo *storage.Catalog, mem *state.CatalogStore, o rsssync.RSSSyncRuntimeOptions) (*rsssync.RSSSyncService, *tmdb.Client, *thetvdb.Client) {
	// Cinemeta-like mode: keep external provider wiring focused on TMDB/TheTVDB only.
	// Legacy clients are intentionally not initialized.
	if !cinemetaDisabled() {
		g := httpclient.NewGetter(o.HTTPTimeout, o.UserAgent, 6<<20)
		o.Cinemeta = cinemeta.NewClient(g, strings.TrimSpace(os.Getenv("GOANIMES_CINEMETA_BASE_URL")))
		if o.CinemetaMinDelay <= 0 {
			if d, err := time.ParseDuration(getenv("GOANIMES_CINEMETA_MIN_DELAY", "250ms")); err == nil {
				o.CinemetaMinDelay = d
			}
		}
	}
	var tmdbCl *tmdb.Client
	if !tmdbDisabled() {
		key := strings.TrimSpace(os.Getenv("GOANIMES_TMDB_API_KEY"))
		if key != "" {
			g := httpclient.NewGetter(o.HTTPTimeout, o.UserAgent, 2<<20)
			tmdbCl = tmdb.NewClient(g, key)
			o.TMDB = tmdbCl
			if o.TMDBMinDelay <= 0 {
				if d, err := time.ParseDuration(getenv("GOANIMES_TMDB_MIN_DELAY", "250ms")); err == nil {
					o.TMDBMinDelay = d
				}
			}
		}
	}
	var tvdbCl *thetvdb.Client
	if !tvdbDisabled() {
		key := strings.TrimSpace(os.Getenv("GOANIMES_TVDB_API_KEY"))
		if key != "" {
			g := httpclient.NewGetter(o.HTTPTimeout, o.UserAgent, 8<<20)
			pin := strings.TrimSpace(os.Getenv("GOANIMES_TVDB_PIN"))
			tvdbCl = thetvdb.NewClient(g, key, pin)
			o.TheTVDB = tvdbCl
			if o.TVDBMinDelay <= 0 {
				if d, err := time.ParseDuration(getenv("GOANIMES_TVDB_MIN_DELAY", "400ms")); err == nil {
					o.TVDBMinDelay = d
				}
			}
		}
	}
	return rsssync.NewRSSSyncService(repo, mem, o, nil), tmdbCl, tvdbCl
}

func tmdbDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GOANIMES_TMDB_DISABLED")))
	return v == "1" || v == "true" || v == "yes"
}

func tvdbDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GOANIMES_TVDB_DISABLED")))
	return v == "1" || v == "true" || v == "yes"
}

func cinemetaDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GOANIMES_CINEMETA_DISABLED")))
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
