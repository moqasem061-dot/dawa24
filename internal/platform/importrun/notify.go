package importrun

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/muhiya/dawa24-store/internal/platform/progress"
)

// Announcing a run's progress the moment it is written.
//
// The alternative was to put a Publish call beside every UpdateProgress in the
// codebase, and there are fourteen of them across three packages, in loops, in
// error paths, and in two different processes. One of them would have been
// missed — the terminal one, most likely, because it is the one that appears
// once — and a bar that never hears "done" is exactly the "stuck at 99%" report
// this work exists to end.
//
// So the announcement lives where the write does. Anything that moves a run
// through the repository is published by definition, including code written
// later by somebody who has never read this file.

// Notifying decorates a Repository so that every state or progress write is
// announced to whoever is watching the run.
//
// It embeds the Repository rather than listing its methods, so a method added
// to the interface later still compiles here and simply does not publish —
// which is the safe direction for a decorator to fail in.
type Notifying struct {
	Repository
	pub *progress.Publisher

	// mu guards meta, which remembers the fields a snapshot needs but
	// UpdateProgress is not given: the public id everything is routed by, and
	// the row total the bar prints beside the percentage.
	//
	// Cached because UpdateProgress is called once per batch of rows on a large
	// import, and re-reading the run each time to learn a UUID that cannot
	// change would add a query to the hot loop of every import on the platform.
	mu   sync.Mutex
	meta map[int64]runMeta
}

type runMeta struct {
	publicID string
	total    int
	// The last values published for this run. TransitionState is not given a
	// percentage — it moves a state — and a snapshot that filled the gap with
	// zero would send the bar back to the start at the exact moment the import
	// finished. Remembering the last figures lets a state change carry the
	// progress that was already true.
	lastPhase     string
	lastPercent   int
	lastProcessed int
}

// WithProgress returns a Repository that publishes as it writes.
//
// A nil publisher returns the repository unchanged, so a deployment without the
// live channel is not a deployment with a broken one.
func WithProgress(repo Repository, pub *progress.Publisher) Repository {
	if repo == nil || pub == nil {
		return repo
	}
	return &Notifying{Repository: repo, pub: pub, meta: make(map[int64]runMeta)}
}

// CreateRun records the run and remembers what a snapshot will need.
func (n *Notifying) CreateRun(ctx context.Context, run *Run) error {
	if err := n.Repository.CreateRun(ctx, run); err != nil {
		return err
	}
	n.remember(run.ID, runMeta{
		publicID: run.PublicID, total: run.TotalRows,
		lastPhase: run.Phase, lastPercent: run.Percent, lastProcessed: run.ProcessedRows,
	})
	n.announce(ctx, run.ID, run.Phase, run.Percent, run.ProcessedRows, run.State, "")
	return nil
}

// UpdateProgress writes the phase and counts, then says so.
func (n *Notifying) UpdateProgress(ctx context.Context, id int64, phase string, percent, processed int) error {
	if err := n.Repository.UpdateProgress(ctx, id, phase, percent, processed); err != nil {
		return err
	}
	n.announce(ctx, id, phase, percent, processed, StateProcessing, "")
	return nil
}

// TransitionState moves the run and announces the new state.
//
// The terminal states are the ones that matter most here: a browser watching
// the stream closes its connection on them, and a bar reaches 100 only because
// one of them said so.
func (n *Notifying) TransitionState(ctx context.Context, id int64, newState string) error {
	if err := n.Repository.TransitionState(ctx, id, newState); err != nil {
		return err
	}
	percent := -1
	if newState == StateCommitted {
		percent = 100
	}
	n.announce(ctx, id, "", percent, -1, newState, "")
	if isTerminal(newState) {
		n.forget(id)
	}
	return nil
}

// FailRun announces the failure with its reason, so the bar can stop where it
// got to and say why instead of freezing.
func (n *Notifying) FailRun(ctx context.Context, id int64, errMsg string) error {
	if err := n.Repository.FailRun(ctx, id, errMsg); err != nil {
		return err
	}
	n.announce(ctx, id, errMsg, -1, -1, StateFailed, errMsg)
	n.forget(id)
	return nil
}

// SetResult refreshes the cached total, because the summary is where a run
// that could not count its rows up front finally states how many there were.
func (n *Notifying) SetResult(ctx context.Context, id int64, result json.RawMessage) error {
	if err := n.Repository.SetResult(ctx, id, result); err != nil {
		return err
	}
	n.forget(id) // the next announce re-reads, picking up the final counts
	return nil
}

// announce builds and publishes one snapshot.
//
// percent and processed are passed as -1 where the caller does not know them,
// in which case the values already on the run are used. That is what lets
// TransitionState publish without claiming a percentage it was never given.
func (n *Notifying) announce(ctx context.Context, id int64, phase string, percent, processed int, state, errMsg string) {
	meta, ok := n.recall(id)
	if !ok {
		// Nothing cached for this run yet. One read, then cached for the rest
		// of it.
		run, err := n.Repository.GetRunByID(ctx, id)
		if err != nil || run == nil {
			return
		}
		meta = runMeta{
			publicID: run.PublicID, total: run.TotalRows,
			lastPhase: run.Phase, lastPercent: run.Percent, lastProcessed: run.ProcessedRows,
		}
	}
	if meta.publicID == "" {
		return
	}

	// Fill anything the caller could not state from what was last true.
	if phase == "" {
		phase = meta.lastPhase
	}
	if percent < 0 {
		percent = meta.lastPercent
	}
	if processed < 0 {
		processed = meta.lastProcessed
	}
	meta.lastPhase, meta.lastPercent, meta.lastProcessed = phase, percent, processed
	n.remember(id, meta)

	n.pub.Publish(ctx, progress.Snapshot{
		ID:      meta.publicID,
		Percent: percent,
		Message: phase,
		Current: processed,
		Total:   meta.total,
		State:   state,
		Done:    isTerminal(state),
		Error:   errMsg,
	})
}

func isTerminal(state string) bool {
	return state == StateCommitted || state == StateFailed || state == StateCancelled
}

func (n *Notifying) remember(id int64, m runMeta) {
	n.mu.Lock()
	n.meta[id] = m
	n.mu.Unlock()
}

func (n *Notifying) recall(id int64) (runMeta, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	m, ok := n.meta[id]
	return m, ok
}

func (n *Notifying) forget(id int64) {
	n.mu.Lock()
	delete(n.meta, id)
	n.mu.Unlock()
}
