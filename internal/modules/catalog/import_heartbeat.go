package catalog

import (
	"context"
	"sync"
	"time"
)

// progressHeartbeat is the longest a healthy run may go without writing to its
// own session row.
//
// The row is the only thing that can tell a crashed run from a slow one: the
// orphan check in SessionProgress reaps a processing session nobody has written
// to for orphanRecoveryDelay. Writes used to happen only when the pipeline
// reported, so a stretch that reports nothing — the catalogue read and index
// build, an AI request waiting on the network — let the row go stale while the
// run was perfectly healthy. A ticker now re-writes the last known position on
// this interval whatever the pipeline is doing, for as long as the run lives.
//
// Fifteen seconds. One narrow UPDATE of one row, four times a minute, while an
// import is actually running.
const progressHeartbeat = 15 * time.Second

// persistedProgress wraps the in-memory reporter so the session row keeps up
// with the run, and returns the function that stops its heartbeat.
//
// Two destinations at two deliberately different rates: the live bar hears
// every tick over the progress hub, and the row is written on a phase change or
// every progressHeartbeat. Taking the bar's movement from the row instead would
// mean a write per batch, or a watcher re-reading on a timer — the polling this
// replaced.
//
// The heartbeat's writes are harmless after the run ends: SaveImportProgress
// only touches a row still in 'processing', so it can never overwrite the
// ready, failed or cancelled state the run finished in.
func (s *Service) persistedProgress(
	ctx context.Context, publicID string, report ProgressFunc,
) (ProgressFunc, func()) {
	var (
		mu        sync.Mutex
		last      ImportProgress
		lastWrite time.Time
	)
	save := func(p ImportProgress) {
		if err := s.imports.SaveImportProgress(ctx, publicID, p); err != nil {
			s.log.DebugContext(ctx, "persist import progress",
				"session", publicID, "phase", p.Phase, "error", err)
		}
	}

	stop := make(chan struct{})
	var once sync.Once
	go func() {
		ticker := time.NewTicker(progressHeartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				mu.Lock()
				due := last.Phase != "" && time.Since(lastWrite) >= progressHeartbeat
				snapshot := last
				if due {
					lastWrite = time.Now()
				}
				mu.Unlock()
				if due {
					save(snapshot)
				}
			}
		}
	}()

	fn := func(phase ImportPhase, current, total int) {
		report.report(phase, current, total)

		p := ImportProgress{Phase: phase, Current: current, Total: total, Message: phase.Label()}
		if s.notifyProgress != nil {
			s.notifyProgress(publicID, p)
		}

		mu.Lock()
		changed := phase != last.Phase
		due := changed || time.Since(lastWrite) >= progressHeartbeat
		last = p
		if due {
			lastWrite = time.Now()
		}
		mu.Unlock()
		if due {
			save(p)
		}
	}
	return fn, func() { once.Do(func() { close(stop) }) }
}
