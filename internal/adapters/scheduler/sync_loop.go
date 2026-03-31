package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/wallissonmarinho/GoAnimes/internal/core/ports"
)

// SyncLoop runs RSS sync on a fixed interval.
type SyncLoop struct {
	Sync     ports.SyncRunner
	Interval time.Duration
	Log      *slog.Logger
}

// Run blocks until ctx is cancelled.
func (l *SyncLoop) Run(ctx context.Context) {
	if l.Log == nil {
		l.Log = slog.Default()
	}
	if l.Interval <= 0 {
		l.Interval = 30 * time.Minute
	}
	t := time.NewTicker(l.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			l.fire()
		}
	}
}

func (l *SyncLoop) fire() {
	defer func() {
		if r := recover(); r != nil {
			l.Log.Error("sync job panic", slog.Any("panic", r))
		}
	}()
	runCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	res := l.Sync.Run(runCtx)
	if len(res.Errors) > 0 {
		l.Log.Warn("sync job warnings",
			slog.String("message", res.Message),
			slog.Any("errors", res.Errors))
	} else {
		l.Log.Info("sync job ok", slog.String("message", res.Message))
	}
}
