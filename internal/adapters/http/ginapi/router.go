package ginapi

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/anilist"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/state"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

// Config holds HTTP settings.
type Config struct {
	AdminAPIKey string
}

// Deps wires handlers.
type Deps struct {
	Sync     ports.SyncRunner
	RSSAdmin ports.RSSAdmin
	Store    *state.CatalogStore
	AniList  *anilist.Client // optional: lazy-fetch synopsis when cache is empty
	Log      *slog.Logger
}

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

	engine.GET("/health", h.getHealth)

	pub := engine.Group("")
	{
		pub.GET("/manifest.json", h.getManifest)
		pub.GET("/catalog/:type/:catalog_id", h.getCatalog)
		pub.GET("/meta/:type/:meta_id", h.getMeta)
		pub.GET("/stream/:type/:stream_id", h.getStream)
	}

	admin := engine.Group("/api/v1")
	admin.Use(adminAuthMiddleware(cfg.AdminAPIKey, d.Log))
	{
		h.registerRSSSourceRoutes(admin)
		admin.POST("/rebuild", h.postRebuild)
		admin.GET("/sync-status", h.getSyncStatus)
	}
}
