package matchflow_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

func TestRunCounts_Reconciliation(t *testing.T) {
	var rc matchflow.RunCounts
	states := []matchflow.State{
		matchflow.StateAccepted,
		matchflow.StateConfirmed,
		matchflow.StateNeedsReview,
		matchflow.StateAmbiguous,
		matchflow.StateNoCandidate,
		matchflow.StateInvalidInput,
		matchflow.StateExcluded,
		matchflow.StateUnavailable,
	}

	for _, s := range states {
		rc.AddState(s)
	}

	if rc.Total != len(states) {
		t.Errorf("Total = %d, want %d", rc.Total, len(states))
	}
	if rc.PrimaryTotal() != rc.Total {
		t.Errorf("PrimaryTotal = %d, want %d (reconciliation mismatch)", rc.PrimaryTotal(), rc.Total)
	}
}
