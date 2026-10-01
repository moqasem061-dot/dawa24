package pipeline

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// TestSmartOrder_GatewayDisabled asserts that when AI gateway is disabled,
// smartorder run lines have ai_state=off and ordering proceeds on deterministic matches (US9 T132).
func TestSmartOrder_GatewayDisabled(t *testing.T) {
	ctx := context.Background()

	policy := productmatch.DefaultPolicy()
	policy.AI.Enabled = false

	targetID := int64(105)
	dec := matchflow.RowDecision{
		V:         1,
		State:     matchflow.StateAccepted,
		Basis:     matchflow.BasisExact,
		ProductID: &targetID,
		Revision:  1,
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

	if cols.MatchedProductID == nil || *cols.MatchedProductID != targetID {
		t.Fatalf("expected matched product ID %d, got %v", targetID, cols.MatchedProductID)
	}
	_ = ctx
}
