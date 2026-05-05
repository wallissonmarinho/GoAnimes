package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wallissonmarinho/GoAnimes/internal/adapters/http/api"
	"github.com/wallissonmarinho/GoAnimes/internal/app"
	"github.com/wallissonmarinho/GoAnimes/internal/app/config"
	"github.com/wallissonmarinho/GoAnimes/internal/observability"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	serviceName := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME"))
	if serviceName == "" {
		serviceName = "goanimes"
	}
	shutdownOtel, err := observability.Setup(ctx, serviceName)
	if err != nil {
		log.Printf("otel setup failed: %v", err)
	} else {
		defer func() {
			_ = shutdownOtel(context.Background())
		}()
	}
	appCtx, err := app.Build(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = app.Shutdown(context.Background(), appCtx)
	}()

	if getenv("GIN_MODE", "") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(corsMiddleware())
	engine.Use(otelgin.Middleware(serviceName, otelgin.WithFilter(func(r *http.Request) bool {
		return r.URL.Path != "/health"
	})))
	api.Register(engine, api.Deps{
		Stremio:  appCtx.Stremio,
		Sync:     appCtx.Sync,
		Admin:    appCtx.Admin,
		AdminKey: cfg.AdminAPIKey,
	})

	addr := listenAddr()
	srv := &http.Server{
		Addr:              addr,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}

func listenAddr() string {
	if p := strings.TrimSpace(os.Getenv("PORT")); p != "" {
		p = strings.TrimPrefix(p, ":")
		return ":" + p
	}
	addr := strings.TrimSpace(os.Getenv("GOANIMES_ADDR"))
	if addr == "" {
		addr = ":8080"
	}
	return addr
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, HEAD")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
