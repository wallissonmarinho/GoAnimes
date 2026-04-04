package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/observability"
	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

// SyncLoop runs full RSS sync on Interval and optionally probes main feeds on PollInterval to trigger an early sync when feeds change.
type SyncLoop struct {
	Sync     ports.SyncRunner
	Interval time.Duration // full rebuild (default 30m)
	// PollInterval when >0 runs RSSMainFeedsChanged on that cadence and calls Run when a main feed changed (default off in struct; main sets 1m).
	PollInterval time.Duration
	Log          *slog.Logger
}

// Run blocks until ctx is cancelled.
func (l *SyncLoop) Run(ctx context.Context) {
	if l.Log == nil {
		l.Log = slog.Default()
	}
	if l.Interval <= 0 {
		l.Interval = 30 * time.Minute
	}
	full := time.NewTicker(l.Interval)
	defer full.Stop()
	var poll *time.Ticker
	if l.PollInterval > 0 {
		poll = time.NewTicker(l.PollInterval)
		defer poll.Stop()
	}
	for {
		if poll == nil {
			select {
			case <-ctx.Done():
				return
			case <-full.C:
				l.fire("sync.scheduled", "sync job")
			}
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-full.C:
			l.fire("sync.scheduled", "sync job")
		case <-poll.C:
			l.pollAndMaybeFire()
		}
	}
}

func (l *SyncLoop) pollAndMaybeFire() {
	defer func() {
		if r := recover(); r != nil {
			l.Log.Error("rss poll panic", slog.Any("panic", r))
		}
	}()
	if l.Sync.SyncRunning() {
		return
	}
	runCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	runCtx, span := observability.StartSyncSpan(runCtx, "sync.rss_poll")
	defer span.End()
	if !l.Sync.RSSMainFeedsChanged(runCtx) {
		return
	}
	if l.Sync.SyncRunning() {
		return
	}
	l.Log.Info("rss main feed(s) changed, running sync")
	l.fire("sync.scheduled", "sync job (rss poll)")
}

func (l *SyncLoop) fire(spanName, okLogPrefix string) {
	defer func() {
		if r := recover(); r != nil {
			l.Log.Error("sync job panic", slog.Any("panic", r))
		}
	}()
	runCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	runCtx, span := observability.StartSyncSpan(runCtx, spanName)
	defer span.End()
	res := l.Sync.Run(runCtx)
	if len(res.Errors) > 0 {
		l.Log.Warn("sync job warnings",
			slog.String("message", res.Message),
			slog.Any("errors", res.Errors))
	} else {
		l.Log.Info(okLogPrefix+" ok", slog.String("message", res.Message))
	}
}
