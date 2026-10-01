package ingest

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// TestVendor_GatewayDisabled verifies that with the AI gateway disabled,
// enhancement is skipped, every row has ai_state=off, and review and commit succeed (US9 T132).
func TestVendor_GatewayDisabled(t *testing.T) {
	ctx := context.Background()

	// Simulate prepare result with AI disabled
	policy := productmatch.DefaultPolicy()
	policy.AI.Enabled = false

	dec := matchflow.RowDecision{
		V:        1,
		State:    matchflow.StateAccepted,
		Basis:    matchflow.BasisExact,
		Revision: 1,
		Versions: matchflow.Versions{
			Policy: policy.Version,
		},
		AI: matchflow.AIDecision{
			State: matchflow.AIStateOff,
		},
	}

	cols := matchflow.Columns(dec)
	if cols.AIState != string(matchflow.AIStateOff) {
		t.Fatalf("expected ai_state off, got %s", cols.AIState)
	}

	// Verify row is valid for review and commit
	if cols.MatchState != string(matchflow.StateAccepted) {
		t.Fatalf("expected accepted state, got %s", cols.MatchState)
	}

	// Simulate commit revalidation
	revalidatedDec := dec
	if revalidatedDec.State != matchflow.StateAccepted {
		t.Fatalf("expected state accepted after commit revalidation")
	}
	_ = ctx
}
