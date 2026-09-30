package pipeline

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder"
)

// mockOfferRepo implements the minimal repository methods needed by Supplier.Resolve.
type mockOfferRepo struct {
	smartorder.Repository
	offers          map[int64][]smartorder.Offer
	storedCandidates map[int64][]smartorder.Candidate
	upsertedSels    []*smartorder.Selection
	queriedProducts [][]int64
}

func (m *mockOfferRepo) LoadOffers(_ context.Context, _ int64, productIDs []int64) ([]smartorder.Offer, error) {
	m.queriedProducts = append(m.queriedProducts, productIDs)
	var out []smartorder.Offer
	for _, pid := range productIDs {
		if offs, ok := m.offers[pid]; ok {
			out = append(out, offs...)
		}
	}
	return out, nil
}

func (m *mockOfferRepo) ReplaceRunCandidates(_ context.Context, _ int64, byLine map[int64][]smartorder.Candidate) (map[int64][]smartorder.Candidate, error) {
	m.storedCandidates = make(map[int64][]smartorder.Candidate)
	out := make(map[int64][]smartorder.Candidate)
	candID := int64(1000)
	for lineID, cands := range byLine {
		assigned := make([]smartorder.Candidate, len(cands))
		for i, c := range cands {
			candID++
			c.ID = candID
			assigned[i] = c
		}
		m.storedCandidates[lineID] = assigned
		out[lineID] = assigned
	}
	return out, nil
}

func (m *mockOfferRepo) UpsertSelections(_ context.Context, selections []*smartorder.Selection) error {
	m.upsertedSels = selections
	return nil
}

// T071: An identified product with no eligible offer has offer_state="identified_no_offer",
// and is never switched to an in-stock sibling product.
func TestIdentifiedProductWithNoEligibleOffer_NeverSwitchedToInStockSibling(t *testing.T) {
	ctx := context.Background()
	const prodA = int64(101) // Requested/identified product
	const prodB = int64(102) // Sibling product in stock

	branchID := int64(68)
	repo := &mockOfferRepo{
		offers: map[int64][]smartorder.Offer{
			// Product A has NO offers available
			prodA: {},
			// Product B (sibling) has ample in-stock offers
			prodB: {
				{
					ProductID:       prodB,
					VariantID:       901,
					VendorOrgID:     188,
					BranchID:        &branchID,
					VariantBranchID: &branchID,
					PriceMinor:      4500,
					MinOrderQty:     1,
					StockQty:        500,
					VendorActive:    true,
					ProductActive:   true,
				},
			},
		},
	}

	gate := &recordingGate{}
	supplier := newTestSupplierWithRepo(repo, gate, 186, 65)

	line := matchedLine(1, prodA, 10)
	line.RunID = 42

	lines := []*smartorder.Line{line}
	total, err := supplier.Resolve(ctx, lines)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// 1. Matched product ID MUST NOT be changed to sibling prodB
	if line.MatchedProductID == nil || *line.MatchedProductID != prodA {
		t.Fatalf("MatchedProductID changed: got %v, want %d (must never switch to sibling)", line.MatchedProductID, prodA)
	}

	// 2. Line offer_state MUST be identified_no_offer
	if line.OfferState != smartorder.OfferStateIdentifiedNoOffer {
		t.Errorf("OfferState: got %q, want %q", line.OfferState, smartorder.OfferStateIdentifiedNoOffer)
	}

	// 3. Line outcome MUST NOT be ordered
	if line.Outcome == smartorder.OutcomeOrdered {
		t.Errorf("Outcome should not be ordered when no eligible offer exists: got %v", line.Outcome)
	}

	// 4. Total should be zero since line was not orderable
	if !total.IsZero() {
		t.Errorf("Expected zero total, got %v", total)
	}

	// 5. Assert repo LoadOffers was only queried for prodA, not prodB
	if len(repo.queriedProducts) != 1 || len(repo.queriedProducts[0]) != 1 || repo.queriedProducts[0][0] != prodA {
		t.Errorf("LoadOffers queried unexpected products: %v, expected [%d]", repo.queriedProducts, prodA)
	}
}

// T071: Test that identified_no_offer is clearly distinct from unmatched lines and offered lines.
func TestIdentifiedNoOffer_DistinctFromUnmatchedAndOffered(t *testing.T) {
	ctx := context.Background()
	const prodIdentifiedNoOffer = int64(201)
	const prodOffered = int64(202)

	branchID := int64(68)
	repo := &mockOfferRepo{
		offers: map[int64][]smartorder.Offer{
			prodIdentifiedNoOffer: {
				// Offer exists but out of stock / ineligible
				{
					ProductID:       prodIdentifiedNoOffer,
					VariantID:       902,
					VendorOrgID:     188,
					BranchID:        &branchID,
					VariantBranchID: &branchID,
					PriceMinor:      3000,
					MinOrderQty:     1,
					StockQty:        0, // Out of stock
					VendorActive:    true,
					ProductActive:   true,
				},
			},
			prodOffered: {
				{
					ProductID:       prodOffered,
					VariantID:       903,
					VendorOrgID:     189,
					BranchID:        &branchID,
					VariantBranchID: &branchID,
					PriceMinor:      3500,
					MinOrderQty:     1,
					StockQty:        50,
					VendorActive:    true,
					ProductActive:   true,
				},
			},
		},
	}

	gate := &recordingGate{
		refuse: map[int64]string{
			902: "insufficient_stock",
		},
	}
	supplier := newTestSupplierWithRepo(repo, gate, 186, 65)

	lineIdentifiedNoOffer := matchedLine(1, prodIdentifiedNoOffer, 5)
	lineIdentifiedNoOffer.RunID = 10

	lineUnmatched := &smartorder.Line{
		ID:           2,
		RunID:        10,
		EffectiveQty: 5,
		Outcome:      smartorder.OutcomeUnmatched,
	}

	lineOffered := matchedLine(3, prodOffered, 2)
	lineOffered.RunID = 10

	lines := []*smartorder.Line{lineIdentifiedNoOffer, lineUnmatched, lineOffered}
	_, err := supplier.Resolve(ctx, lines)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Line 1: Identified with no eligible offer
	if lineIdentifiedNoOffer.OfferState != smartorder.OfferStateIdentifiedNoOffer {
		t.Errorf("line 1 OfferState: got %q, want %q", lineIdentifiedNoOffer.OfferState, smartorder.OfferStateIdentifiedNoOffer)
	}
	if lineIdentifiedNoOffer.MatchedProductID == nil || *lineIdentifiedNoOffer.MatchedProductID != prodIdentifiedNoOffer {
		t.Errorf("line 1 MatchedProductID lost: got %v", lineIdentifiedNoOffer.MatchedProductID)
	}

	// Line 2: Unmatched line (never identified to any catalogue product)
	if lineUnmatched.OfferState != "" {
		t.Errorf("unmatched line should have empty OfferState, got %q", lineUnmatched.OfferState)
	}
	if lineUnmatched.Outcome != smartorder.OutcomeUnmatched {
		t.Errorf("unmatched line outcome: got %q, want %q", lineUnmatched.Outcome, smartorder.OutcomeUnmatched)
	}

	// Line 3: Offered line (identified and has eligible offer)
	if lineOffered.OfferState != smartorder.OfferStateOffered {
		t.Errorf("line 3 OfferState: got %q, want %q", lineOffered.OfferState, smartorder.OfferStateOffered)
	}
	if lineOffered.Outcome != smartorder.OutcomeOrdered {
		t.Errorf("line 3 Outcome: got %q, want %q", lineOffered.Outcome, smartorder.OutcomeOrdered)
	}
}

func newTestSupplierWithRepo(repo smartorder.Repository, gate smartorder.AvailabilityGate, buyerOrgID, buyerBranchID int64) *Supplier {
	s := NewSupplier(repo, stubCoverage{}, nil,
		&smartorder.Config{OrganizationID: buyerOrgID},
		BranchLocation{BranchID: buyerBranchID, Lat: 30.04, Lng: 31.23, HasCoord: true})
	s.SetAvailabilityGate(gate)
	return s
}

func TestOfferSelection_OnlyReceivesAcceptedOrConfirmedProducts(t *testing.T) {
	ctx := context.Background()
	const prodAccepted = int64(101)
	const prodNeedsReview = int64(102)
	const prodConfirmed = int64(103)

	branchID := int64(68)
	repo := &mockOfferRepo{
		offers: map[int64][]smartorder.Offer{
			prodAccepted: {
				{
					ProductID:       prodAccepted,
					VariantID:       901,
					VendorOrgID:     188,
					BranchID:        &branchID,
					VariantBranchID: &branchID,
					PriceMinor:      4500,
					MinOrderQty:     1,
					StockQty:        500,
					VendorActive:    true,
					ProductActive:   true,
				},
			},
			prodNeedsReview: {
				{
					ProductID:       prodNeedsReview,
					VariantID:       902,
					VendorOrgID:     188,
					BranchID:        &branchID,
					VariantBranchID: &branchID,
					PriceMinor:      3000,
					MinOrderQty:     1,
					StockQty:        500,
					VendorActive:    true,
					ProductActive:   true,
				},
			},
			prodConfirmed: {
				{
					ProductID:       prodConfirmed,
					VariantID:       903,
					VendorOrgID:     188,
					BranchID:        &branchID,
					VariantBranchID: &branchID,
					PriceMinor:      2000,
					MinOrderQty:     1,
					StockQty:        500,
					VendorActive:    true,
					ProductActive:   true,
				},
			},
		},
	}

	gate := &recordingGate{}
	supplier := newTestSupplierWithRepo(repo, gate, 186, 65)

	l1 := matchedLine(1, prodAccepted, 5)
	l1.MatchState = "accepted"

	l2 := matchedLine(2, prodNeedsReview, 5)
	l2.MatchState = "needs_review" // unaccepted candidate

	l3 := matchedLine(3, prodConfirmed, 2)
	l3.MatchState = "confirmed"

	lines := []*smartorder.Line{l1, l2, l3}
	_, err := supplier.Resolve(ctx, lines)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// repo.LoadOffers should only have been queried for prodAccepted (101) and prodConfirmed (103), NOT prodNeedsReview (102)
	if len(repo.queriedProducts) == 0 {
		t.Fatalf("expected LoadOffers call")
	}
	queried := repo.queriedProducts[0]
	for _, pid := range queried {
		if pid == prodNeedsReview {
			t.Errorf("offer selection loaded offers for unaccepted product %d", prodNeedsReview)
		}
	}

	// Line 2 outcome must be unmatched, OfferState empty, product ID unchanged
	if l2.Outcome != smartorder.OutcomeUnmatched {
		t.Errorf("unaccepted line outcome: got %s, want unmatched", l2.Outcome)
	}
	if l2.OfferState != "" {
		t.Errorf("unaccepted line offer_state: got %s, want empty", l2.OfferState)
	}
	if l2.MatchedProductID == nil || *l2.MatchedProductID != prodNeedsReview {
		t.Errorf("l2 MatchedProductID changed unexpectedly: %v", l2.MatchedProductID)
	}

	// Line 1 and 3 are ordered
	if l1.OfferState != smartorder.OfferStateOffered {
		t.Errorf("l1 OfferState: got %s, want offered", l1.OfferState)
	}
	if l3.OfferState != smartorder.OfferStateOffered {
		t.Errorf("l3 OfferState: got %s, want offered", l3.OfferState)
	}
}
