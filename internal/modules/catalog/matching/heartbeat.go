package matching

import (
	"context"
	"time"
)

// DefaultHeartbeatInterval is the standard frequency for updating run heartbeats (10s per US12).
const DefaultHeartbeatInterval = 10 * time.Second

// StartHeartbeat starts an asynchronous ticker goroutine that periodically calls repo.Heartbeat
// to keep match_runs.heartbeat_at fresh while a worker is preparing or enhancing a run.
// It returns a cancel function that halts the ticker and waits for the goroutine to terminate.
func StartHeartbeat(ctx context.Context, repo Repository, runID int64, interval time.Duration) func() {
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-stopCh:
				return
			case <-ticker.C:
				if repo != nil && runID > 0 {
					_ = repo.Heartbeat(ctx, runID)
				}
			}
		}
	}()

	return func() {
		select {
		case <-stopCh:
			// Already stopped
		default:
			close(stopCh)
		}
		<-doneCh
	}
}
