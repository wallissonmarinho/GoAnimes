package app

import (
	"context"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/rss"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/storage/mongo"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/tmdbapi"
	"github.com/wallissonmarinho/GoAnimes/internal/app/admin"
	"github.com/wallissonmarinho/GoAnimes/internal/app/config"
	"github.com/wallissonmarinho/GoAnimes/internal/app/stremio"
	syncsvc "github.com/wallissonmarinho/GoAnimes/internal/app/sync"
)

type App struct {
	Store   *mongo.Store
	Sync    *syncsvc.Service
	Stremio *stremio.Service
	Admin   *admin.Service
}

func Build(ctx context.Context, cfg config.Config) (*App, error) {
	store, err := mongo.Connect(ctx, cfg.MongoURI, cfg.MongoDB)
	if err != nil {
		return nil, err
	}
	catalogRepo := mongo.NewCatalogRepository(store)
	feedRepo := mongo.NewFeedRepository(store)
	mappingRepo := mongo.NewMappingRepository(store)
	reader := rss.NewReader()
	var tmdbClient *tmdbapi.Client
	if cfg.TMDBAPIKey != "" {
		tmdbClient = tmdbapi.NewClient(cfg.TMDBAPIKey, cfg.HTTPTimeout)
	}
	guard := &syncsvc.Guard{}
	syncService := &syncsvc.Service{
		Feeds:   feedRepo,
		Mapping: mappingRepo,
		Catalog: catalogRepo,
		Reader:  reader,
		TMDB:    tmdbClient,
		Guard:   guard,
	}
	stremioService := &stremio.Service{Repo: catalogRepo, TMDB: tmdbClient}
	adminService := &admin.Service{Feeds: feedRepo, Mapping: mappingRepo, Catalog: catalogRepo}
	return &App{Store: store, Sync: syncService, Stremio: stremioService, Admin: adminService}, nil
}

func Shutdown(ctx context.Context, app *App) error {
	if app == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return mongo.Disconnect(ctx, app.Store)
}
