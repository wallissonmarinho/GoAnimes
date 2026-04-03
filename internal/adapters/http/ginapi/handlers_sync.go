package ginapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *handlers) postRebuild(c *gin.Context) {
	if h.deps.Sync == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sync unavailable"})
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		res := h.deps.Sync.Run(ctx)
		if h.deps.Log != nil && len(res.Errors) > 0 {
			h.deps.Log.Warn("rebuild warnings", "errors", res.Errors)
		}
	}()
	c.JSON(http.StatusAccepted, gin.H{"status": "accepted"})
}

func (h *handlers) getSyncStatus(c *gin.Context) {
	snap, err := h.deps.Catalog.LoadSyncStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	errs := snap.LastSyncErrors
	if errs == nil {
		errs = []string{}
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":          snap.OK,
		"message":     snap.Message,
		"item_count":  snap.ItemCount,
		"started_at":  snap.StartedAt,
		"finished_at": snap.FinishedAt,
		"errors":      errs,
	})
}
