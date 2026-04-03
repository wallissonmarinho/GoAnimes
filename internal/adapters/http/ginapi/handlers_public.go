package ginapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// registerPublic attaches health and Stremio catalog routes (no admin).
func (h *handlers) registerPublic(engine *gin.Engine) {
	engine.GET("/health", h.getHealth)
	engine.HEAD("/health", h.headHealth)

	pub := engine.Group("")
	{
		pub.GET("/manifest.json", h.getManifest)
		// Stremio: /catalog/anime/goanimes.json or .../goanimes/genre=Fantasia.json
		pub.GET("/catalog/:type/*catalogPath", h.getCatalog)
		pub.GET("/meta/:type/:meta_id", h.getMeta)
		pub.GET("/stream/:type/:stream_id", h.getStream)
	}
}

func (h *handlers) getHealth(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

func (h *handlers) headHealth(c *gin.Context) {
	c.Status(http.StatusOK)
}
