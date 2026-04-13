package ginapi

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/anilist"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/jikan"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/kitsu"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/thetvdb"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/tmdb"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

// Config holds HTTP settings.
type Config struct {
	AdminAPIKey string
}

// Deps wires handlers.
type Deps struct {
	Sync    ports.SyncRunner
	Catalog ports.CatalogAdmin // RSS admin, sync-status from DB, live Stremio catalog (memory), persistence after lazy enrich
	AniList *anilist.Client    // optional: lazy-fetch when cache is empty
	Jikan   *jikan.Client      // optional: MAL fallback when AniList left gaps
	Kitsu   *kitsu.Client      // optional: Kitsu JSON:API when gaps remain after Jikan
	TMDB    *tmdb.Client       // optional: backdrops via IMDb→TMDB find (GOANIMES_TMDB_API_KEY; GOANIMES_TMDB_DISABLED)
	TheTVDB *thetvdb.Client    // optional: fanart via TheTVDB v4 (GOANIMES_TVDB_API_KEY; GOANIMES_TVDB_DISABLED)
	// SynopsisTrans optional (nil in tests); production wires gilang Google translate.
	SynopsisTrans ports.SynopsisTranslator
	Log           *slog.Logger
	// SyncStatusLocation when non-nil converts started_at/finished_at in GET /sync-status to this IANA zone (stored UTC in DB).
	SyncStatusLocation *time.Location
	// SyncRunTimeout is the context deadline for RSSSyncService.Run on POST rebuild (GOANIMES_SYNC_RUN_TIMEOUT; zero → 30m).
	SyncRunTimeout time.Duration
	// GoaiAuditAdmin optional; when nil, GoAI admin routes return 503.
	GoaiAuditAdmin ports.GoAIAuditAdmin
}

// handlers binds Gin routes to ports. See handlers_public.go, handlers_admin.go, handlers_stremio.go, handlers_rss.go, handlers_sync.go, middleware.go.
type handlers struct {
	cfg  Config
	deps Deps
}

func newHandlers(cfg Config, d Deps) *handlers {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &handlers{cfg: cfg, deps: d}
}

// Register attaches routes to the engine.
func Register(engine *gin.Engine, cfg Config, d Deps) {
	h := newHandlers(cfg, d)
	h.registerPublic(engine)
	h.registerAdminV1(engine)
}
