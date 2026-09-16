package smartorder

import (
	"fmt"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// The run lifecycle.
//
//	draft ──▶ mapping ──▶ queued ──▶ processing ──▶ completed ──▶ finalizing ──▶ placed
//	                                                    ▲   │
//	                                                    └───┴──▶ stale
//	  (any non-terminal) ──▶ failed
//
// Two transitions carry the weight here.
//
// `completed → stale` exists because a buyer can change the delivery branch,
// the criteria or the tolerance after results are on screen. Those inputs decide
// which suppliers were eligible and which won, so the results no longer describe
// the configuration the buyer is looking at. Rather than silently recompute
// (slow, and the screen would change under them) or silently not (wrong), the
// run is marked stale and finalisation is withheld until it is re-run.
//
// `finalizing → placed` is one-way and guarded by FinalizedAt. Placing an order
// twice from one double-clicked button is the failure that costs a pharmacy real
// money, so the guard lives in the domain and is re-checked inside the
// transaction that writes the order.

var allowedTransitions = map[RunStatus][]RunStatus{
	StatusDraft:      {StatusMapping, StatusFailed},
	StatusMapping:    {StatusQueued, StatusMapping, StatusFailed},
	StatusQueued:     {StatusProcessing, StatusFailed},
	StatusProcessing: {StatusCompleted, StatusFailed},
	StatusCompleted:  {StatusFinalizing, StatusStale, StatusQueued, StatusFailed},
	StatusStale:      {StatusQueued, StatusFailed},
	StatusFinalizing: {StatusPlaced, StatusCompleted, StatusFailed},
	StatusPlaced:     {},
	StatusFailed:     {StatusQueued},
}

// stepFor is the wizard step a status belongs to, so a returning buyer lands
// where the run actually is rather than where their browser last was.
var stepFor = map[RunStatus]int{
	StatusDraft:      1,
	StatusMapping:    2,
	StatusQueued:     3,
	StatusProcessing: 3,
	StatusCompleted:  4,
	StatusStale:      4,
	StatusFinalizing: 5,
	StatusPlaced:     5,
	StatusFailed:     3,
}

// Terminal reports whether a status admits no further progress.
func (s RunStatus) Terminal() bool { return s == StatusPlaced }

// Step returns the wizard step this status corresponds to.
func (s RunStatus) Step() int {
	if step, ok := stepFor[s]; ok {
		return step
	}
	return 1
}

// CanTransitionTo reports whether a status change is legal.
func (s RunStatus) CanTransitionTo(next RunStatus) bool {
	for _, allowed := range allowedTransitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

// TransitionTo moves the run to next, refusing anything the lifecycle forbids.
//
// A run that already carries FinalizedAt is refused outright: an order exists,
// and no later state change may create a second one.
func (r *Run) TransitionTo(next RunStatus) error {
	if r.FinalizedAt != nil && next != StatusPlaced {
		return apperr.Conflict("smartorder.already_finalized",
			fmt.Sprintf("run %s was finalized and cannot move to %s", r.RunNumber, next))
	}
	if r.Status == next && next != StatusMapping {
		return nil
	}
	if !r.Status.CanTransitionTo(next) {
		return apperr.Conflict("smartorder.invalid_transition",
			fmt.Sprintf("cannot move run %s from %s to %s", r.RunNumber, r.Status, next))
	}
	r.Status = next
	r.CurrentStep = next.Step()
	return nil
}

// MarkStale records that the configuration moved after results were produced.
//
// Results are kept: they are still the best information available, and throwing
// them away would cost the buyer a re-run they may not want. They are simply no
// longer finalisable until refreshed.
func (r *Run) MarkStale() error {
	if r.Status != StatusCompleted {
		return nil // nothing to invalidate
	}
	return r.TransitionTo(StatusStale)
}

// CanFinalize reports whether the run may be turned into an order, and why not
// when it may not.
func (r *Run) CanFinalize() error {
	if r.FinalizedAt != nil {
		return apperr.Conflict("smartorder.already_finalized",
			fmt.Sprintf("run %s already produced order", r.RunNumber))
	}
	switch r.Status {
	case StatusCompleted:
		return nil
	case StatusStale:
		return apperr.Conflict("smartorder.stale",
			"settings changed since these results were produced; re-run before finalizing")
	default:
		return apperr.Conflict("smartorder.not_ready",
			fmt.Sprintf("run %s is %s and cannot be finalized", r.RunNumber, r.Status))
	}
}
