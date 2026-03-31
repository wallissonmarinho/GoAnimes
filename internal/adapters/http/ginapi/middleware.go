package ginapi

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func adminAuthMiddleware(apiKey string, lg *slog.Logger) gin.HandlerFunc {
	key := strings.TrimSpace(apiKey)
	return func(c *gin.Context) {
		if key == "" {
			if lg != nil {
				lg.Warn("admin API key unset — /api/v1 admin routes are open")
			}
			c.Next()
			return
		}
		auth := c.GetHeader("Authorization")
		const p = "Bearer "
		if strings.HasPrefix(auth, p) && strings.TrimSpace(auth[len(p):]) == key {
			c.Next()
			return
		}
		if c.GetHeader("X-Admin-API-Key") == key {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
	}
}

// CorsMiddleware sets CORS headers and answers OPTIONS with 204. Use as the first engine middleware
// so browser preflight works for Stremio addon URLs.
func CorsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, X-Requested-With")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
