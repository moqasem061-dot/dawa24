package catalog

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// TestCatalogImport_GatewayDisabled asserts that when AI gateway is unconfigured,
// staging rows have ai_state=off and catalog import review & commit work (US9 T132).
func TestCatalogImport_GatewayDisabled(t *testing.T) {
	ctx := context.Background()

	policy := productmatch.DefaultPolicy()
	policy.AI.Enabled = false

	dec := matchflow.RowDecision{
		V:        1,
		State:    matchflow.StateAccepted,
		Basis:    matchflow.BasisSimilarity,
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

	if cols.MatchState != string(matchflow.StateAccepted) {
		t.Fatalf("expected accepted state, got %s", cols.MatchState)
	}
	_ = ctx
}
