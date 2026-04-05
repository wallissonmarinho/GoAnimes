package ports

import (
	"context"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/core/domain"
)

// SyncRunner runs RSS fetch + parse + filter.
type SyncRunner interface {
	Run(ctx context.Context) domain.SyncResult
	// SyncRunning is true while Run is executing (interval job or manual rebuild).
	SyncRunning() bool
	// SyncRunStartedAt is UTC start of the in-progress Run, or zero if not running.
	SyncRunStartedAt() time.Time
	// RSSMainFeedsChanged conditional-GETs each configured top-level feed URL (not Erai per-anime feeds).
	// Returns true when a feed is new, its body changed vs the last probe/full sync, or a feed URL was removed from config.
	// Callers should skip while SyncRunning.
	RSSMainFeedsChanged(ctx context.Context) bool
}
