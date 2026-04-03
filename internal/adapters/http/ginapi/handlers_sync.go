package ginapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

// syncStatusIdleJSON is returned when no sync job is in progress (fields use omitempty — no null placeholders).
type syncStatusIdleJSON struct {
	SyncRunning bool       `json:"sync_running"`
	OK          bool       `json:"ok"`
	Message     string     `json:"message,omitempty"`
	ItemCount   int        `json:"item_count"`
	Errors      []string   `json:"errors,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

// syncStatusPreviousJSON is the last persisted snapshot (DB) while a new Run is executing.
type syncStatusPreviousJSON struct {
	OK         bool       `json:"ok"`
	Message    string     `json:"message,omitempty"`
	ItemCount  int        `json:"item_count"`
	Errors     []string   `json:"errors,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// syncStatusRunningJSON is returned while Run holds the sync lock.
type syncStatusRunningJSON struct {
	SyncRunning  bool                   `json:"sync_running"`
	StartedAt    *time.Time             `json:"started_at,omitempty"`
	PreviousSync syncStatusPreviousJSON `json:"previous_sync"`
}

func timePtrUTC(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	utc := t.UTC()
	return &utc
}

func syncErrorsFromSnapshot(snap domain.CatalogSnapshot) []string {
	errs := snap.LastSyncErrors
	if len(errs) == 0 {
		return nil
	}
	return errs
}

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
	errs := syncErrorsFromSnapshot(snap)

	running := false
	if h.deps.Sync != nil {
		running = h.deps.Sync.SyncRunning()
	}

	if running {
		var started *time.Time
		if h.deps.Sync != nil {
			if t := h.deps.Sync.SyncRunStartedAt(); !t.IsZero() {
				started = timePtrUTC(t)
			}
		}
		prev := syncStatusPreviousJSON{
			OK:        snap.OK,
			Message:   snap.Message,
			ItemCount: snap.ItemCount,
			Errors:    errs,
			StartedAt: timePtrUTC(snap.StartedAt),
			FinishedAt: timePtrUTC(snap.FinishedAt),
		}
		c.JSON(http.StatusOK, syncStatusRunningJSON{
			SyncRunning:  true,
			StartedAt:    started,
			PreviousSync: prev,
		})
		return
	}

	out := syncStatusIdleJSON{
		SyncRunning: false,
		OK:          snap.OK,
		Message:     snap.Message,
		ItemCount:   snap.ItemCount,
		Errors:      errs,
		StartedAt:   timePtrUTC(snap.StartedAt),
		FinishedAt:  timePtrUTC(snap.FinishedAt),
	}
	c.JSON(http.StatusOK, out)
}
