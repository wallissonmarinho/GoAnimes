package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	ginapi "github.com/wallissonmarinho/GoAnimes/internal/adapters/http/ginapi"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/observability"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/scheduler"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/state"
	"github.com/wallissonmarinho/GoAnimes/internal/app"
	"github.com/wallissonmarinho/GoAnimes/internal/core/services"
)

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "migrate" {
		os.Exit(runMigrateCLI(os.Args[2:]))
	}

	otelShutdown, lg, err := observability.Setup(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	slog.SetDefault(lg)

	dataDir := getenv("GOANIMES_DATA_DIR", "./data")
	if mkErr := os.MkdirAll(dataDir, 0o755); mkErr != nil {
		slog.Error("data dir", slog.Any("err", mkErr))
		os.Exit(1)
	}
	dbPath := filepath.Join(dataDir, "goanimes.db")
	sqliteDSN := getenv("GOANIMES_SQLITE_DSN", "file:"+dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = sqliteDSN
	}

	cat, err := app.OpenCatalog(dsn)
	if err != nil {
		slog.Error("open catalog", slog.Any("err", err))
		os.Exit(1)
	}
	defer cat.Close()

	mem := &state.CatalogStore{}
	app.HydrateCatalogStore(context.Background(), cat, mem)

	httpTimeout := durationEnv("GOANIMES_HTTP_TIMEOUT", 45*time.Second)
	maxBody := int64Env("GOANIMES_MAX_BODY_BYTES", 50<<20)
	ua := getenv("GOANIMES_USER_AGENT", "GoAnimes/1.0")
	synopsisTr := app.NewSynopsisTranslator(httpTimeout, ua, maxBody)
	slog.Info("synopsis translation", slog.String("translator", synopsisTr.Name()))
	syncSvc, anilistClient, jikanClient, kitsuClient := app.NewRSSSyncService(cat, mem, services.RSSSyncRuntimeOptions{
		HTTPTimeout:   httpTimeout,
		MaxBodyBytes:  maxBody,
		UserAgent:     ua,
		SynopsisTrans: synopsisTr,
	})
	catalogAdmin := app.NewCatalogAdmin(cat, mem)

	if getenv("GIN_MODE", "") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	engine.Use(ginapi.CorsMiddleware())
	engine.Use(gin.Recovery())
	serviceName := getenv("OTEL_SERVICE_NAME", "goanimes")
	observability.RegisterGin(engine, serviceName)
	ginapi.Register(engine, ginapi.Config{AdminAPIKey: app.AdminAPIKey()}, ginapi.Deps{
		Sync:          syncSvc,
		Catalog:       catalogAdmin,
		AniList:       anilistClient,
		Jikan:         jikanClient,
		Kitsu:         kitsuClient,
		SynopsisTrans: synopsisTr,
		Log:           lg,
	})

	addr := listenAddr()
	srv := &http.Server{
		Addr:              addr,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	syncInterval := durationEnv("GOANIMES_SYNC_INTERVAL", 30*time.Minute)
	schedCtx, schedCancel := context.WithCancel(context.Background())
	defer schedCancel()
	loop := &scheduler.SyncLoop{Sync: syncSvc, Interval: syncInterval, Log: lg}
	go loop.Run(schedCtx)

	go func() {
		slog.Info("listening", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server", slog.Any("err", err))
			os.Exit(1)
		}
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("initial sync panic", slog.Any("panic", r))
			}
		}()
		time.Sleep(2 * time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		ctx, span := observability.StartSyncSpan(ctx, "sync.initial")
		defer span.End()
		res := syncSvc.Run(ctx)
		if len(res.Errors) > 0 {
			slog.Warn("initial sync warnings", slog.Any("errors", res.Errors))
		}
		slog.Info("initial sync", slog.String("message", res.Message))
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	slog.Info("shutting down")
	schedCancel()
	shCtx, shCancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer shCancel()
	_ = srv.Shutdown(shCtx)
	_ = otelShutdown(shCtx)
}

func listenAddr() string {
	if p := os.Getenv("PORT"); p != "" {
		return ":" + p
	}
	return getenv("GOANIMES_ADDR", ":8080")
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func int64Env(k string, def int64) int64 {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func durationEnv(k string, def time.Duration) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
