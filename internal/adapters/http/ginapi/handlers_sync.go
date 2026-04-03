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
	running := false
	if h.deps.Sync != nil {
		running = h.deps.Sync.SyncRunning()
	}
	// While Run() is in progress, DB still holds the *previous* completed snapshot.
	// started_at is the live run start; finished_at stays null until persist.
	// ok/message/item_count/errors at top level are only for the last *completed* run — when a job is in flight they are null and the same data is under last_sync_*.
	resp := gin.H{
		"sync_running": running,
	}
	if running {
		resp["ok"] = nil
		resp["message"] = nil
		resp["item_count"] = nil
		resp["errors"] = nil
		resp["finished_at"] = nil
		curStart := time.Time{}
		if h.deps.Sync != nil {
			curStart = h.deps.Sync.SyncRunStartedAt()
		}
		if !curStart.IsZero() {
			resp["started_at"] = curStart
		} else {
			resp["started_at"] = nil
		}
		resp["last_sync_ok"] = snap.OK
		resp["last_sync_message"] = snap.Message
		resp["last_sync_item_count"] = snap.ItemCount
		resp["last_sync_errors"] = errs
		if !snap.StartedAt.IsZero() {
			resp["last_sync_started_at"] = snap.StartedAt
		}
		if !snap.FinishedAt.IsZero() {
			resp["last_sync_finished_at"] = snap.FinishedAt
		}
	} else {
		resp["ok"] = snap.OK
		resp["message"] = snap.Message
		resp["item_count"] = snap.ItemCount
		resp["errors"] = errs
		resp["started_at"] = snap.StartedAt
		resp["finished_at"] = snap.FinishedAt
	}
	c.JSON(http.StatusOK, resp)
}
