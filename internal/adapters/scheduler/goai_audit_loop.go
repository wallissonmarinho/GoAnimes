package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/observability"
)

// GoaiAuditRunner runs a single GoAI audit pass (implemented by services.GoaiAuditWorker.Run).
type GoaiAuditRunner interface {
	Run(ctx context.Context)
}

// GoaiAuditLoop ticks at Interval and invokes Runner.Run (stops entire run on GoAI errors inside worker).
type GoaiAuditLoop struct {
	Runner   GoaiAuditRunner
	Interval time.Duration
	Log      *slog.Logger
}

// Run blocks until ctx is cancelled.
func (l *GoaiAuditLoop) Run(ctx context.Context) {
	if l.Log == nil {
		l.Log = slog.Default()
	}
	if l.Runner == nil || l.Interval <= 0 {
		l.Log.Warn("goai audit loop: disabled (nil runner or interval<=0)")
		<-ctx.Done()
		return
	}
	t := time.NewTicker(l.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			l.fire(ctx)
		}
	}
}

func (l *GoaiAuditLoop) fire(parent context.Context) {
	if l.Log == nil {
		l.Log = slog.Default()
	}
	ctx, span := observability.StartSyncSpan(parent, "goai_audit.tick")
	defer span.End()
	l.Log.Info("goai audit: scheduled run")
	l.Runner.Run(ctx)
}
