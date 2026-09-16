package importrun_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/importrun"
)

func TestRun_StatePredicates(t *testing.T) {
	tests := []struct {
		state     string
		isDone    bool
		isWorking bool
	}{
		{importrun.StateQueued, false, false},
		{importrun.StateProcessing, false, true},
		{importrun.StateReady, false, false},
		{importrun.StateCommitting, false, true},
		{importrun.StateCommitted, true, false},
		{importrun.StateFailed, true, false},
		{importrun.StateCancelled, true, false},
	}

	for _, tc := range tests {
		r := &importrun.Run{State: tc.state}
		if got := r.IsDone(); got != tc.isDone {
			t.Errorf("state %q: IsDone() = %v, want %v", tc.state, got, tc.isDone)
		}
		if got := r.IsWorking(); got != tc.isWorking {
			t.Errorf("state %q: IsWorking() = %v, want %v", tc.state, got, tc.isWorking)
		}
	}
}

func TestProgressFromRun(t *testing.T) {
	run := &importrun.Run{
		State:         importrun.StateProcessing,
		Phase:         "matching rows",
		Percent:       42,
		ProcessedRows: 100,
		TotalRows:     250,
		ErrorMessage:  "",
	}

	prog := importrun.ProgressFromRun(run)
	if prog.State != importrun.StateProcessing {
		t.Errorf("expected state %s, got %s", importrun.StateProcessing, prog.State)
	}
	if prog.Phase != "matching rows" {
		t.Errorf("expected phase %q, got %q", "matching rows", prog.Phase)
	}
	if prog.Percent != 42 {
		t.Errorf("expected percent 42, got %d", prog.Percent)
	}
	if prog.Processed != 100 {
		t.Errorf("expected processed 100, got %d", prog.Processed)
	}
	if prog.Total != 250 {
		t.Errorf("expected total 250, got %d", prog.Total)
	}
	if prog.Done {
		t.Errorf("expected done to be false")
	}
}
