package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
)

// Corporate Operations at the supplier stage.
//
// This stage used to decide it for itself, asking "does the buyer organisation
// hold one of this PRODUCT's institutional works?" — a different question from
// the one commerce.CheckAvailability asks at checkout, which is whether the
// buyer's BRANCH is connected to a branch of the supplier. Because the two
// disagreed, a run's review screen could show a line as orderable and checkout
// would then refuse the whole order at the last click.
//
// The rule now lives in commerce and reaches this stage only through
// AvailabilityGate, so the branch-to-branch reasoning is tested where it is
// implemented (internal/modules/org and internal/modules/commerce). What is
// left to pin here is this stage's own contract, and it is worth pinning
// because getting it wrong reproduces the same defect from the other side: ask
// the gate the whole page's question at once, key its answers by variant, and
// carry a refusal onto the line in words the buyer can act on while they are
// still reviewing.

// recordingGate captures what the pipeline asks and answers from a fixture.
type recordingGate struct {
	batches [][]smartorder.GateLine
	// refuse maps a variant id onto the commerce reason to answer with. A
	// variant that is absent is allowed.
	refuse map[int64]string
}

func (g *recordingGate) Check(_ context.Context, buyerOrgID, buyerBranchID int64,
	_ time.Time, lines []smartorder.GateLine) (map[int64]smartorder.GateVerdict, error) {

	g.batches = append(g.batches, lines)
	out := make(map[int64]smartorder.GateVerdict, len(lines))
	for _, l := range lines {
		if reason, blocked := g.refuse[l.VariantID]; blocked {
			out[l.VariantID] = smartorder.GateVerdict{Allowed: false, Reason: reason}
			continue
		}
		out[l.VariantID] = smartorder.GateVerdict{Allowed: true, MaxQuantity: l.Quantity}
	}
	return out, nil
}

func (g *recordingGate) calls() int {
	n := 0
	for _, b := range g.batches {
		n += len(b)
	}
	return n
}

type stubCoverage struct{}

func (stubCoverage) Serves(context.Context, int64, time.Weekday, float64, float64) (bool, int, error) {
	return true, 100, nil
}

// The gate is the only thing this stage asks, and it is asked with the buyer's
// own identity rather than anything carried on the offer. An offer is data from
// the catalogue; who is buying is not negotiable by it.
func TestGateIsAskedWithTheBuyersOwnIdentity(t *testing.T) {
	vendorBranch := int64(68)
	gate := &recordingGate{}

	s := newTestSupplier(gate, 186, 65)
	line := matchedLine(1, 10, 5)

	offers := []smartorder.Offer{{
		ProductID: 10, VariantID: 900, VendorOrgID: 187,
		BranchID: &vendorBranch, VariantBranchID: &vendorBranch,
		PriceMinor: 5000, MinOrderQty: 1, StockQty: 50,
		VendorActive: true, ProductActive: true,
	}}

	if _, err := s.buildCandidates(context.Background(), line, offers,
		map[int64]coverageVerdict{}, map[instKey]bool{}); err != nil {
		t.Fatalf("buildCandidates: %v", err)
	}

	if gate.calls() != 1 {
		t.Fatalf("expected one gate line, got %d", gate.calls())
	}
	got := gate.batches[0][0]
	if got.VariantID != 900 {
		t.Errorf("variant: got %d, want 900", got.VariantID)
	}
	if got.VendorOrgID != 187 {
		t.Errorf("vendor org: got %d, want 187", got.VendorOrgID)
	}
	if got.Quantity != 5 {
		t.Errorf("quantity: got %d, want 5 — the gate must judge the amount actually being ordered", got.Quantity)
	}
}

// One question for the page, not one per row. The stage batches because a file
// of ten thousand lines touching a few dozen suppliers must not become ten
// thousand availability checks.
func TestGateIsAskedOncePerBatchNotPerOffer(t *testing.T) {
	branchA, branchB := int64(68), int64(70)
	gate := &recordingGate{}
	s := newTestSupplier(gate, 186, 65)

	offers := []smartorder.Offer{
		{ProductID: 10, VariantID: 900, VendorOrgID: 187, BranchID: &branchA, VariantBranchID: &branchA,
			PriceMinor: 5000, MinOrderQty: 1, StockQty: 50, VendorActive: true, ProductActive: true},
		{ProductID: 10, VariantID: 901, VendorOrgID: 188, BranchID: &branchB, VariantBranchID: &branchB,
			PriceMinor: 4000, MinOrderQty: 1, StockQty: 50, VendorActive: true, ProductActive: true},
		{ProductID: 11, VariantID: 902, VendorOrgID: 187, BranchID: &branchA, VariantBranchID: &branchA,
			PriceMinor: 5500, MinOrderQty: 1, StockQty: 50, VendorActive: true, ProductActive: true},
	}

	if _, err := s.buildCandidates(context.Background(), matchedLine(1, 10, 5), offers,
		map[int64]coverageVerdict{}, map[instKey]bool{}); err != nil {
		t.Fatalf("buildCandidates: %v", err)
	}

	if len(gate.batches) != 1 {
		t.Fatalf("expected one batched question, got %d", len(gate.batches))
	}
	if gate.calls() != 3 {
		t.Fatalf("expected all three offers in the one batch, got %d", gate.calls())
	}
}

// A refusal keeps its meaning on the way through. commerce says
// branch_institutional_mismatch; the review screen has to say Corporate
// Operations, not "no supplier", or the buyer goes looking for a sourcing
// problem that does not exist.
func TestInstitutionalRefusalSurvivesTheMapping(t *testing.T) {
	branch := int64(68)
	gate := &recordingGate{refuse: map[int64]string{900: "branch_institutional_mismatch"}}
	s := newTestSupplier(gate, 186, 65)

	line := matchedLine(1, 10, 5)
	offers := []smartorder.Offer{{
		ProductID: 10, VariantID: 900, VendorOrgID: 187,
		BranchID: &branch, VariantBranchID: &branch,
		PriceMinor: 5000, MinOrderQty: 1, StockQty: 50,
		VendorActive: true, ProductActive: true,
	}}

	candidates, err := s.buildCandidates(context.Background(), line, offers,
		map[int64]coverageVerdict{}, map[instKey]bool{})
	if err != nil {
		t.Fatalf("buildCandidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].Eligible {
		t.Fatalf("a refused offer must not stay eligible: %+v", candidates)
	}
	if candidates[0].IneligibleReason != smartorder.ReasonInstitutional {
		t.Errorf("reason: got %q, want %q", candidates[0].IneligibleReason, smartorder.ReasonInstitutional)
	}

	outcome, _ := smartorder.OutcomeFor(true, line.EffectiveQty, candidates)
	if outcome != smartorder.OutcomeInstitutionalBlocked {
		t.Fatalf("outcome: got %q, want %q — the buyer must be told this is a Corporate Operations "+
			"restriction while reviewing, not at the last click", outcome, smartorder.OutcomeInstitutionalBlocked)
	}
}

// Two suppliers of the same product get their own answers. Keying the verdicts
// by product rather than by variant answered once and applied the first
// supplier's verdict to everyone else selling it.
func TestEachSupplierGetsItsOwnVerdict(t *testing.T) {
	branchA, branchB := int64(68), int64(70)
	gate := &recordingGate{refuse: map[int64]string{901: "branch_institutional_mismatch"}}
	s := newTestSupplier(gate, 186, 65)

	offers := []smartorder.Offer{
		{ProductID: 10, VariantID: 900, VendorOrgID: 187, BranchID: &branchA, VariantBranchID: &branchA,
			PriceMinor: 5000, MinOrderQty: 1, StockQty: 50, VendorActive: true, ProductActive: true},
		{ProductID: 10, VariantID: 901, VendorOrgID: 188, BranchID: &branchB, VariantBranchID: &branchB,
			PriceMinor: 4000, MinOrderQty: 1, StockQty: 50, VendorActive: true, ProductActive: true},
	}

	got, err := s.buildCandidates(context.Background(), matchedLine(1, 10, 5), offers,
		map[int64]coverageVerdict{}, map[instKey]bool{})
	if err != nil {
		t.Fatalf("buildCandidates: %v", err)
	}
	if !got[0].Eligible {
		t.Error("the connected supplier must stay eligible")
	}
	if got[1].Eligible {
		t.Error("the unconnected supplier must be refused")
	}
}

// The degenerate gate is still the product-work intersection, for a deployment
// that has no org service to ask. It is no longer on the purchase path, but the
// adapter remains and its behaviour is worth pinning.
func TestSimpleGateStillReadsTheProductsOwnWorks(t *testing.T) {
	gate := smartorder.SimpleInstitutionalGate([]int64{2, 3})

	unrestricted, err := gate.Visible(context.Background(), smartorder.InstitutionalCheck{})
	if err != nil || !unrestricted {
		t.Fatalf("an unrestricted product must be visible: %v %v", unrestricted, err)
	}
	held, _ := gate.Visible(context.Background(), smartorder.InstitutionalCheck{ProductWorkIDs: []int64{3}})
	if !held {
		t.Error("a product restricted to a held work must be visible")
	}
	notHeld, _ := gate.Visible(context.Background(), smartorder.InstitutionalCheck{ProductWorkIDs: []int64{9}})
	if notHeld {
		t.Error("a product restricted to a work the buyer does not hold must be refused")
	}
}

func newTestSupplier(gate smartorder.AvailabilityGate, buyerOrgID, buyerBranchID int64) *Supplier {
	s := NewSupplier(nil, stubCoverage{}, nil,
		&smartorder.Config{OrganizationID: buyerOrgID},
		BranchLocation{BranchID: buyerBranchID, Lat: 30.04, Lng: 31.23, HasCoord: true})
	s.SetAvailabilityGate(gate)
	return s
}

func matchedLine(id, productID int64, qty float64) *smartorder.Line {
	pid := productID
	return &smartorder.Line{ID: id, MatchedProductID: &pid, EffectiveQty: qty}
}
