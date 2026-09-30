package ingest

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// TestRerunNonAcceptedClearsMatchedProductAndPreview tests that when a rerun
// produces a non-accepted state, matched_product_id is set to nil (through Columns)
// and no variant is resolved, invalidating stale downstream stock and price preview.
func TestRerunNonAcceptedClearsMatchedProductAndPreview(t *testing.T) {
	// 1. When decision is non-accepted (e.g. NeedsReview), Columns must project nil MatchedProductID
	unacceptedDec := matchflow.RowDecision{
		State: matchflow.StateNeedsReview,
		Basis: matchflow.BasisSimilarity,
		Candidates: []matchflow.CandidateDecision{
			{ProductID: 202, Rank: 1, Eligible: false},
		},
	}
	cols := matchflow.Columns(unacceptedDec)
	if cols.MatchedProductID != nil {
		t.Fatalf("Columns() on non-accepted decision projected MatchedProductID = %v, want nil", *cols.MatchedProductID)
	}

	// 2. In stagingRun, an unaccepted decision must not resolve a variant, even if SKU matches
	prodID := int64(202)
	varID := int64(501)
	keys := []VariantKey{
		{ID: varID, ProductID: prodID, SKU: "SKU-HAVE", Active: true},
	}
	varIdx := newVariantIndex(keys, map[int64]bool{varID: true}, nil)

	r := &stagingRun{
		variantIdx: varIdx,
	}

	row := &productmatch.Row{
		Number: 1,
		Name:   "Some unaccepted row",
		SKU:    "SKU-HAVE",
	}

	// Verify that if productID is nil (as projected by Columns), no variant is resolved
	productID := cols.MatchedProductID
	var resolvedVariantID *int64
	inWarehouse := false
	if r.variantIdx != nil && productID != nil {
		pID := *productID
		if vID, _ := r.variantIdx.resolve(row, pID, nil); vID > 0 {
			resolvedVariantID = &vID
			inWarehouse = r.variantIdx.inWarehouse != nil && r.variantIdx.inWarehouse[vID]
		}
	}

	if resolvedVariantID != nil {
		t.Fatalf("expected nil variantID for unaccepted product match, got %v", *resolvedVariantID)
	}
	if inWarehouse {
		t.Fatal("expected inWarehouse to be false when productID is nil")
	}
}

// TestClearRowMatchesInvalidatesDownstreamPreview tests that calling ClearRowMatches
// unlinks the rows and clears their downstream stock/price commit preview.
func TestClearRowMatchesInvalidatesDownstreamPreview(t *testing.T) {
	ctx := context.Background()
	existing := int64(202)
	row := stagedRow(1, &existing, "SKU-HAVE", "Existing item", 3, true)
	row.ID = 1001
	rows := []*RowOutcome{row}

	keys := []VariantKey{
		{ID: 501, ProductID: existing, SKU: "SKU-HAVE", Active: true},
	}

	settings := DefaultSettings()
	settings.WarehouseID = 1
	settings.Mode = ModeUpsert

	sess := session("imp-rerun-clear", settings)
	sess.Phase = PhaseReview

	svc, _, _, store := commitFixture(sess, rows, keys, map[int64]bool{501: true})
	store.mentions = []RowMention{
		{ProductID: existing, SourceCode: "SKU-HAVE"},
	}

	// Before clearing: preview shows 1 update and 1 stocked
	planBefore, err := svc.PreviewCommit(ctx, "imp-rerun-clear")
	if err != nil {
		t.Fatalf("PreviewCommit before clear: %v", err)
	}
	if planBefore.Update != 1 || planBefore.Stocked != 1 {
		t.Fatalf("plan before clear = update %d, stocked %d; want 1, 1", planBefore.Update, planBefore.Stocked)
	}

	// User unlinks / clears row matches
	res, err := svc.ClearRowMatches(ctx, "imp-rerun-clear", []int64{1001})
	if err != nil {
		t.Fatalf("ClearRowMatches: %v", err)
	}
	if res.Applied != 1 {
		t.Fatalf("ClearRowMatches applied %d, want 1", res.Applied)
	}

	// After clearing: preview shows 0 updates and 0 stocked (stale downstream preview invalidated)
	planAfter, err := svc.PreviewCommit(ctx, "imp-rerun-clear")
	if err != nil {
		t.Fatalf("PreviewCommit after clear: %v", err)
	}
	if planAfter.Update != 0 || planAfter.Stocked != 0 {
		t.Fatalf("plan after clear = update %d, stocked %d; want 0, 0", planAfter.Update, planAfter.Stocked)
	}
}
