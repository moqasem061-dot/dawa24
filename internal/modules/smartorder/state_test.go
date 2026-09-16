package smartorder

import (
	"testing"
	"time"
)

func TestHappyPathTransitions(t *testing.T) {
	r := &Run{RunNumber: "SO-2026-000001", Status: StatusDraft}

	for _, next := range []RunStatus{
		StatusMapping, StatusQueued, StatusProcessing, StatusCompleted, StatusFinalizing, StatusPlaced,
	} {
		if err := r.TransitionTo(next); err != nil {
			t.Fatalf("transition to %s failed: %v", next, err)
		}
		if r.Status != next {
			t.Fatalf("expected status %s, got %s", next, r.Status)
		}
	}
	if r.CurrentStep != 5 {
		t.Fatalf("a placed run sits on step 5, got %d", r.CurrentStep)
	}
}

func TestCannotSkipAhead(t *testing.T) {
	r := &Run{RunNumber: "SO-2026-000002", Status: StatusDraft}
	if err := r.TransitionTo(StatusPlaced); err == nil {
		t.Fatal("a draft run must not jump straight to placed")
	}
	if r.Status != StatusDraft {
		t.Fatalf("a refused transition must not mutate the run, got %s", r.Status)
	}
}

// FR-050. This is the one that costs a pharmacy real money if it regresses.
func TestFinalizedRunCannotBeFinalizedAgain(t *testing.T) {
	now := time.Now()
	orderID := int64(9001)
	r := &Run{
		RunNumber:   "SO-2026-000003",
		Status:      StatusPlaced,
		FinalizedAt: &now,
		OrderID:     &orderID,
	}

	if err := r.CanFinalize(); err == nil {
		t.Fatal("a run that already produced an order must refuse to finalize again")
	}
	if err := r.TransitionTo(StatusFinalizing); err == nil {
		t.Fatal("a finalized run must not re-enter finalizing")
	}
}

func TestFinalizedGuardHoldsEvenIfStatusLooksReady(t *testing.T) {
	// Defence in depth: a status of `completed` with FinalizedAt set is an
	// inconsistent row, and the guard must still refuse rather than trust status.
	now := time.Now()
	r := &Run{RunNumber: "SO-2026-000004", Status: StatusCompleted, FinalizedAt: &now}

	if err := r.CanFinalize(); err == nil {
		t.Fatal("FinalizedAt must veto finalisation regardless of status")
	}
}

func TestStaleRunRefusesFinalisationUntilRerun(t *testing.T) {
	r := &Run{RunNumber: "SO-2026-000005", Status: StatusCompleted}

	if err := r.CanFinalize(); err != nil {
		t.Fatalf("a completed run should be finalisable: %v", err)
	}
	if err := r.MarkStale(); err != nil {
		t.Fatalf("marking stale failed: %v", err)
	}
	if r.Status != StatusStale {
		t.Fatalf("expected stale, got %s", r.Status)
	}
	if err := r.CanFinalize(); err == nil {
		t.Fatal("a stale run must not be finalisable until it is re-run")
	}
	// Re-running is the way out.
	if err := r.TransitionTo(StatusQueued); err != nil {
		t.Fatalf("a stale run must be re-runnable: %v", err)
	}
}

func TestMarkStaleOnlyAppliesToCompletedRuns(t *testing.T) {
	// Changing settings mid-processing is not staleness; the run has not yet
	// produced results to invalidate.
	r := &Run{RunNumber: "SO-2026-000006", Status: StatusProcessing}
	if err := r.MarkStale(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Status != StatusProcessing {
		t.Fatalf("expected processing to be left alone, got %s", r.Status)
	}
}

func TestFailedRunCanBeRetried(t *testing.T) {
	r := &Run{RunNumber: "SO-2026-000007", Status: StatusFailed}
	if err := r.TransitionTo(StatusQueued); err != nil {
		t.Fatalf("a failed run should be retryable: %v", err)
	}
}

func TestStepMapping(t *testing.T) {
	cases := map[RunStatus]int{
		StatusDraft:      1,
		StatusMapping:    2,
		StatusQueued:     3,
		StatusProcessing: 3,
		StatusCompleted:  4,
		StatusStale:      4,
		StatusFinalizing: 5,
		StatusPlaced:     5,
	}
	for status, want := range cases {
		if got := status.Step(); got != want {
			t.Errorf("%s: expected step %d, got %d", status, want, got)
		}
	}
}

func TestNotReadyRunReportsWhy(t *testing.T) {
	r := &Run{RunNumber: "SO-2026-000008", Status: StatusProcessing}
	err := r.CanFinalize()
	if err == nil {
		t.Fatal("a processing run is not finalisable")
	}
	// The message must name the state, so the UI can say something better than
	// "an error occurred".
	if got := err.Error(); got == "" {
		t.Fatal("expected an explanatory error")
	}
}
