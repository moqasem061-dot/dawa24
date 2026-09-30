package pipeline

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// T072: Requested pack 30 vs an offered unit of 10 without a relationship
// blocks the line for review (offer_state="blocked_pack");
// with a relationship factor 3, the converted quantity is exact and priced with money.Amount.
func TestPackConvert_WithoutRelationship_BlocksForReview(t *testing.T) {
	ctx := context.Background()
	const prodID = int64(301)
	branchID := int64(68)

	repo := &mockOfferRepo{
		offers: map[int64][]smartorder.Offer{
			prodID: {
				{
					ProductID:       prodID,
					VariantID:       910,
					VendorOrgID:     188,
					BranchID:        &branchID,
					VariantBranchID: &branchID,
					PriceMinor:      1500, // 15.00 EGP
					Unit:            "10 أقراص",
					MinOrderQty:     1,
					StockQty:        100,
					VendorActive:    true,
					ProductActive:   true,
				},
			},
		},
	}

	gate := &recordingGate{}
	supplier := newTestSupplierWithRepo(repo, gate, 186, 65)

	// Line requested pack 30
	line := &smartorder.Line{
		ID:               1,
		RunID:            55,
		RowNumber:        1,
		RawName:          "بروفين 400 مجم 30 قرص",
		MatchedProductID: &[]int64{prodID}[0],
		EffectiveQty:     1,
	}

	lines := []*smartorder.Line{line}
	total, err := supplier.Resolve(ctx, lines)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// 1. Without relationship: line must be blocked for review
	if line.OfferState != smartorder.OfferStateBlockedPack {
		t.Errorf("OfferState: got %q, want %q", line.OfferState, smartorder.OfferStateBlockedPack)
	}

	if line.Outcome == smartorder.OutcomeOrdered {
		t.Errorf("Outcome should not be ordered without pack relationship: got %v", line.Outcome)
	}

	if !total.IsZero() {
		t.Errorf("Expected zero total for blocked pack line, got %v", total)
	}
}

func TestPackConvert_WithRelationshipFactor3_ExactConversion(t *testing.T) {
	ctx := context.Background()
	const prodID = int64(302)
	branchID := int64(68)

	repo := &mockOfferRepo{
		offers: map[int64][]smartorder.Offer{
			prodID: {
				{
					ProductID:       prodID,
					VariantID:       911,
					VendorOrgID:     188,
					BranchID:        &branchID,
					VariantBranchID: &branchID,
					PriceMinor:      1500, // 15.00 EGP per 10-pack unit
					Unit:            "10 أقراص",
					MinOrderQty:     1,
					StockQty:        100,
					VendorActive:    true,
					ProductActive:   true,
				},
			},
		},
	}

	gate := &recordingGate{}
	supplier := newTestSupplierWithRepo(repo, gate, 186, 65)

	// Approved pack relationship: factor 3 (e.g. 1 pack of 30 = 3 units of 10)
	supplier.SetPackRelationships([]productmatch.PackRelationship{
		{
			ID:        1,
			ProductID: prodID,
			FromUnit:  "30",
			ToUnit:    "10",
			Factor:    3,
		},
	})

	// Line requested 2 packs of 30
	line := &smartorder.Line{
		ID:               2,
		RunID:            55,
		RowNumber:        2,
		RawName:          "بروفين 400 مجم 30 قرص",
		MatchedProductID: &[]int64{prodID}[0],
		EffectiveQty:     2,
	}

	lines := []*smartorder.Line{line}
	total, err := supplier.Resolve(ctx, lines)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// 1. With relationship: line must be offered and ordered
	if line.OfferState != smartorder.OfferStateOffered {
		t.Errorf("OfferState: got %q, want %q", line.OfferState, smartorder.OfferStateOffered)
	}

	if line.Outcome != smartorder.OutcomeOrdered {
		t.Errorf("Outcome: got %q, want %q", line.Outcome, smartorder.OutcomeOrdered)
	}

	// 2. Converted quantity: 2 packs of 30 * factor 3 = 6 units of 10
	storedCands := repo.storedCandidates[line.ID]
	if len(storedCands) != 1 {
		t.Fatalf("Expected 1 stored candidate, got %d", len(storedCands))
	}
	if storedCands[0].ConvertedQty != 6 {
		t.Errorf("ConvertedQty: got %d, want 6", storedCands[0].ConvertedQty)
	}

	// 3. Selection line net must be exact money.Amount: 6 * 15.00 EGP = 90.00 EGP (9000 minor)
	if len(repo.upsertedSels) != 1 {
		t.Fatalf("Expected 1 selection, got %d", len(repo.upsertedSels))
	}
	wantNet := money.FromMinor(9000)
	if repo.upsertedSels[0].LineNet != wantNet {
		t.Errorf("LineNet: got %v, want %v", repo.upsertedSels[0].LineNet, wantNet)
	}

	// 4. Order total must match exactly
	if total != wantNet {
		t.Errorf("Total: got %v, want %v", total, wantNet)
	}
}
