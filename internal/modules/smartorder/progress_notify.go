package smartorder

import (
	"context"
	"strconv"
	"sync"

	"github.com/muhiya/dawa24-store/internal/platform/progress"
)

// Announcing a smart order's progress as it is recorded.
//
// The buyer's screen asked the server for a number TWICE A SECOND for the whole
// run, and a run over a large basket takes minutes. That is several hundred
// requests, each one re-reading the run's entire event list to compute a
// percentage that had usually not changed.
//
// It publishes instead. Progress is written through one method (AppendEvent)
// and the run's end through one more (UpdateRunStatus), so decorating the
// repository makes every one of them announce itself without touching the
// pipeline.

// ProgressKey is the hub key one run is published under.
//
// The INTERNAL run id. An Event carries only that — it has no public id on it —
// and the stream endpoint has to load the run for authorisation anyway, so it
// derives the same key from the row it already holds.
func ProgressKey(runID int64) string {
	return "smartorder:" + strconv.FormatInt(runID, 10)
}

// notifyingRepository publishes progress as it is written.
//
// It embeds the interface rather than listing its methods, so one added later
// still compiles and simply does not publish.
type notifyingRepository struct {
	Repository
	pub *progress.Publisher

	// mu guards seen, which holds the running maximum percentage and latest
	// caption per run.
	//
	// RunPercent is max(event.Percent()) over every event of the run, and a
	// publisher that only ever sees ONE event cannot compute a maximum without
	// remembering. Keeping it here reproduces that exactly, and reproduces the
	// property that matters most: the figure never goes backwards, so a late
	// event from an earlier stage cannot rewind the bar.
	mu   sync.Mutex
	seen map[int64]runProgress
}

type runProgress struct {
	percent int
	stage   Stage
}

// WithProgressNotifications returns a repository that publishes as it writes.
//
// A nil publisher returns the repository unchanged, so a deployment without the
// live channel keeps working on the JSON poll beside the stream.
func WithProgressNotifications(repo Repository, pub *progress.Publisher) Repository {
	if repo == nil || pub == nil {
		return repo
	}
	return &notifyingRepository{
		Repository: repo,
		pub:        pub,
		seen:       make(map[int64]runProgress),
	}
}

// AppendEvent records one step of a run and says how far it has got.
func (n *notifyingRepository) AppendEvent(ctx context.Context, e *Event) error {
	if err := n.Repository.AppendEvent(ctx, e); err != nil {
		return err
	}
	if e == nil || e.RunID == 0 {
		return nil
	}

	n.mu.Lock()
	state := n.seen[e.RunID]
	if p := e.Percent(); p > state.percent {
		state.percent = p
	}
	if e.Stage != "" {
		state.stage = e.Stage
	}
	n.seen[e.RunID] = state
	n.mu.Unlock()

	// Never 100 from here. The bar reaches the end because the run's STATUS
	// said so, not because the arithmetic of a stage got there while the
	// finalise step was still writing.
	percent := state.percent
	if percent >= 100 {
		percent = 99
	}

	snap := progress.Snapshot{
		ID:      ProgressKey(e.RunID),
		Percent: percent,
		State:   string(StatusProcessing),
	}
	if state.stage != "" {
		snap.Message = state.stage.Label()
	}
	if e.Processed != nil {
		snap.Current = *e.Processed
	}
	if e.Total != nil {
		snap.Total = *e.Total
	}
	n.pub.Publish(ctx, snap)
	return nil
}

// UpdateRunStatus carries the terminal states, which are the ones the watching
// screen is actually waiting for.
func (n *notifyingRepository) UpdateRunStatus(ctx context.Context, id int64, status RunStatus, step int, failureReason string) error {
	if err := n.Repository.UpdateRunStatus(ctx, id, status, step, failureReason); err != nil {
		return err
	}
	if !status.Terminal() {
		return nil
	}

	n.mu.Lock()
	state := n.seen[id]
	delete(n.seen, id) // the run is over; stop remembering it
	n.mu.Unlock()

	percent := 100
	if status == StatusFailed {
		// A failed run keeps whatever it had reached. Filling the bar implies
		// it finished; zeroing it implies it never started.
		percent = state.percent
	}
	n.pub.Publish(ctx, progress.Snapshot{
		ID:      ProgressKey(id),
		Percent: percent,
		Message: failureReason,
		State:   string(status),
		Done:    true,
		Error:   failureReason,
	})
	return nil
}
