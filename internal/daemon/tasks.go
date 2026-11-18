package daemon

import (
	"context"
	"log/slog"
	"time"
)

// Task sweep cadence bounds.
const (
	maxSweepInterval = time.Minute
	minSweepInterval = 100 * time.Millisecond
)

// sweepInterval is how often the task lease sweeper runs: every minute, or a
// quarter of the idle threshold when that is shorter, so small decay
// settings (tests, demos) still decay promptly.
func sweepInterval(idleAfter time.Duration) time.Duration {
	return max(min(maxSweepInterval, idleAfter/4), minSweepInterval)
}

// startTaskSweeper runs the task lease sweeper until ctx is canceled; the
// returned channel closes when it has stopped, which must happen before the
// store closes.
func (d *daemon) startTaskSweeper(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	interval := sweepInterval(d.tasks.Decay().IdleAfter)
	go func() {
		defer close(done)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				n, err := d.tasks.Sweep(ctx)
				if err != nil && ctx.Err() == nil {
					d.logger.LogAttrs(ctx, slog.LevelWarn, "task sweep failed", slog.String("error", err.Error()))
				}
				if n > 0 {
					d.logger.LogAttrs(ctx, slog.LevelInfo, "abandoned decayed tasks", slog.Int("count", n))
				}
			}
		}
	}()
	return done
}
