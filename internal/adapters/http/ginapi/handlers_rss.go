package ginapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

type sourceCreateBody struct {
	URL   string `json:"url"`
	Label string `json:"label"`
}

func (h *handlers) registerRSSSourceRoutes(admin *gin.RouterGroup) {
	admin.POST("/rss-sources", h.postRSSSource)
	admin.GET("/rss-sources", h.listRSSSources)
	admin.DELETE("/rss-sources/:id", h.deleteRSSSource)
}

func (h *handlers) postRSSSource(c *gin.Context) {
	var body sourceCreateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	created, err := h.deps.RSSAdmin.CreateRSSSource(c.Request.Context(), body.URL, body.Label)
	if err != nil {
		if errors.Is(err, domain.ErrDuplicateRSSSourceURL) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, domain.ErrInvalidSourceURL) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (h *handlers) listRSSSources(c *gin.Context) {
	list, err := h.deps.RSSAdmin.ListRSSSources(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *handlers) deleteRSSSource(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	if delErr := h.deps.RSSAdmin.DeleteRSSSource(c.Request.Context(), id); delErr != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": delErr.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
