package observability

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// RegisterGin adds OpenTelemetry tracing and structured access logs (slog).
func RegisterGin(engine *gin.Engine, serviceName string) {
	engine.Use(otelgin.Middleware(serviceName, otelgin.WithFilter(func(r *http.Request) bool {
		// Liveness/readiness: não criar span (evita poluir Jaeger).
		return r.URL.Path != "/health"
	})))
	engine.Use(accessLogMiddleware())
}

func accessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/health" {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		lg := slog.Default()
		if lg == nil {
			return
		}
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		lg.Log(c.Request.Context(), slog.LevelInfo, "http_request",
			slog.String("http.method", c.Request.Method),
			slog.String("http.route", route),
			slog.Int("http.status_code", c.Writer.Status()),
			slog.String("url.path", c.Request.URL.Path),
			slog.Duration("duration", time.Since(start)),
		)
	}
}
