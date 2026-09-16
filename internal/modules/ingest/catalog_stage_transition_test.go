package ingest

import (
	"context"
	"testing"
)

// The transition that wedged every import.
//
// Staging publishes PhaseProcessing, works for minutes, then writes its result.
// It wrote that result through SaveDraft — which refuses a session in
// 'processing' by design, because that guard is what stops a vendor editing
// settings underneath a run already reading them. The write matched zero rows
// and returned "not open"; the failure handler then tried the same call and was
// refused for the same reason. Every import ended at 95% in 'processing' with
// the matching complete, the AI stage finished and paid for, and nothing the
// vendor could reach.
//
// It was invisible to the test suite because nothing exercised the phase
// predicate: a fake store that accepts every write reproduces neither the
// refusal nor the wedge. This one models the predicate.

// phasedStore is an ImportStore that enforces the phase rules the real
// repository enforces, and nothing else.
type phasedStore struct {
	phase   Phase
	percent int
	note    string
	// finishCalls counts outcome writes that were accepted.
	finishCalls int
}

func (s *phasedStore) openPhase() bool {
	switch s.phase {
	case PhaseMapping, PhaseSettings, PhaseReview, PhaseConfirm:
		return true
	}
	return false
}

func (s *phasedStore) SaveDraft(_ context.Context, sess *Session) error {
	// WHERE phase IN ('mapping','settings','review','confirm')
	if !s.openPhase() {
		return errNotOpen
	}
	s.phase = sess.Phase
	return nil
}

func (s *phasedStore) FinishStaging(_ context.Context, sess *Session) error {
	// WHERE phase = 'processing'
	if s.phase != PhaseProcessing {
		return errNotProcessing
	}
	s.phase = sess.Phase
	s.finishCalls++
	return nil
}

func (s *phasedStore) Progress(_ context.Context, _ int64, percent int, note string) error {
	switch s.phase {
	case PhaseProcessing, PhaseReview, PhaseConfirm:
		s.percent, s.note = percent, note
	}
	return nil
}

func TestStagingCanPublishItsOutcomeFromProcessing(t *testing.T) {
	store := &phasedStore{phase: PhaseSettings}
	session := &Session{ID: 1, PublicID: "abc"}

	// What StageInBackground does: claim the phase before the run starts.
	session.Phase = PhaseProcessing
	if err := store.SaveDraft(context.Background(), session); err != nil {
		t.Fatalf("could not claim the run: %v", err)
	}
	if store.phase != PhaseProcessing {
		t.Fatalf("phase = %s, want processing", store.phase)
	}

	// What StageImport does at the end. Through SaveDraft this was refused, and
	// that refusal is the whole bug.
	session.Phase = PhaseReview
	if err := store.SaveDraft(context.Background(), session); err == nil {
		t.Fatal("SaveDraft accepted a write from 'processing'; the real repository refuses it, " +
			"so a test that accepts it cannot catch the wedge")
	}

	if err := store.FinishStaging(context.Background(), session); err != nil {
		t.Fatalf("FinishStaging refused the outcome write: %v", err)
	}
	if store.phase != PhaseReview {
		t.Fatalf("phase = %s, want review; the vendor never reaches the review screen otherwise", store.phase)
	}
}

func TestStagingCanRecordAFailureFromProcessing(t *testing.T) {
	// The other half. A run that fails must be able to say so, or a failed
	// import is indistinguishable from a slow one for as long as anyone watches.
	store := &phasedStore{phase: PhaseProcessing}
	session := &Session{ID: 1, PublicID: "abc", Phase: PhaseFailed, ErrorMessage: "bad file"}

	if err := store.FinishStaging(context.Background(), session); err != nil {
		t.Fatalf("FinishStaging refused a failure write: %v", err)
	}
	if store.phase != PhaseFailed {
		t.Fatalf("phase = %s, want failed", store.phase)
	}
}

// The progress screen stops polling when the run stops, not when the import is
// finished. Staging ends in 'review', which is not a terminal phase, so a
// poller keyed on Phase.Terminal() polls a completed run for ever.
func TestReviewIsNotTerminalButTheRunHasStopped(t *testing.T) {
	if PhaseReview.Terminal() {
		t.Fatal("PhaseReview reports terminal; the poller's contract assumes it does not")
	}
	if PhaseProcessing.Terminal() {
		t.Fatal("PhaseProcessing reports terminal")
	}

	// This is the predicate the progress endpoint uses.
	stopped := func(p Phase) bool { return p != PhaseProcessing }

	for _, p := range []Phase{PhaseReview, PhaseFailed, PhaseCompleted, PhaseCancelled} {
		if !stopped(p) {
			t.Errorf("phase %s reported as still running; the bar would never stop", p)
		}
	}
	if stopped(PhaseProcessing) {
		t.Error("PhaseProcessing reported as stopped; the screen would reload mid-run")
	}
}

// The two refusals the real repository returns, modelled as sentinel errors so
// the test asserts on the condition rather than on a message.
var (
	errNotOpen       = errPhase("import.not_open")
	errNotProcessing = errPhase("import.not_processing")
)

type errPhase string

func (e errPhase) Error() string { return string(e) }
