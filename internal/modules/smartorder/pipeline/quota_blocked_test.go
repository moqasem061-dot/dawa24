package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
)

type mockAvailabilityGate struct {
	verdicts map[int64]smartorder.GateVerdict
	calls    [][]smartorder.GateLine
}

func (m *mockAvailabilityGate) Check(ctx context.Context, buyerOrgID, buyerBranchID int64, when time.Time,
	lines []smartorder.GateLine) (map[int64]smartorder.GateVerdict, error) {
	m.calls = append(m.calls, lines)
	return m.verdicts, nil
}

func TestQuotaBlockedWhenAvailabilityGateReturnsQuota(t *testing.T) {
	gate := &mockAvailabilityGate{
		verdicts: map[int64]smartorder.GateVerdict{
			900: {Allowed: false, Reason: "quota"},
		},
	}

	branch := int64(68)
	s := NewSupplier(nil, stubCoverage{}, nil,
		&smartorder.Config{OrganizationID: 186},
		BranchLocation{BranchID: 65, Lat: 30.04, Lng: 31.23, HasCoord: true})
	s.SetAvailabilityGate(gate)

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

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	c := candidates[0]
	if c.Eligible {
		t.Errorf("candidate should not be eligible when quota blocked")
	}
	if c.IneligibleReason != smartorder.ReasonQuota {
		t.Errorf("ineligible reason: got %q, want %q", c.IneligibleReason, smartorder.ReasonQuota)
	}

	outcome, reason := smartorder.OutcomeFor(true, line.EffectiveQty, candidates)
	if outcome != smartorder.OutcomeQuotaBlocked {
		t.Fatalf("outcome: got %q, want %q", outcome, smartorder.OutcomeQuotaBlocked)
	}
	if reason != smartorder.ReasonQuota {
		t.Fatalf("reason: got %q, want %q", reason, smartorder.ReasonQuota)
	}
}

func TestMissingBranchLocationFailsClosedWhenGateReportsNoLocation(t *testing.T) {
	gate := &mockAvailabilityGate{
		verdicts: map[int64]smartorder.GateVerdict{
			900: {Allowed: false, Reason: "branch_no_location"},
		},
	}

	branch := int64(68)
	s := NewSupplier(nil, stubCoverage{}, nil,
		&smartorder.Config{OrganizationID: 186},
		BranchLocation{BranchID: 65, HasCoord: false})
	s.SetAvailabilityGate(gate)

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

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	c := candidates[0]
	if c.Eligible {
		t.Errorf("candidate should not be eligible when branch has no location")
	}
	if c.IneligibleReason != smartorder.ReasonNoLocation {
		t.Errorf("ineligible reason: got %q, want %q", c.IneligibleReason, smartorder.ReasonNoLocation)
	}

	outcome, reason := smartorder.OutcomeFor(true, line.EffectiveQty, candidates)
	if outcome != smartorder.OutcomeCoverageBlocked {
		t.Fatalf("outcome: got %q, want %q", outcome, smartorder.OutcomeCoverageBlocked)
	}
	if reason != smartorder.ReasonNoLocation {
		t.Fatalf("reason: got %q, want %q", reason, smartorder.ReasonNoLocation)
	}
}

func TestAvailabilityGateSelectsAllowedCandidateOverQuotaCandidate(t *testing.T) {
	gate := &mockAvailabilityGate{
		verdicts: map[int64]smartorder.GateVerdict{
			901: {Allowed: false, Reason: "quota"},
			902: {Allowed: true},
		},
	}

	branch := int64(68)
	s := NewSupplier(nil, stubCoverage{}, nil,
		&smartorder.Config{OrganizationID: 186},
		BranchLocation{BranchID: 65, Lat: 30.04, Lng: 31.23, HasCoord: true})
	s.SetAvailabilityGate(gate)

	line := matchedLine(1, 10, 5)
	offers := []smartorder.Offer{
		{
			ProductID: 10, VariantID: 901, VendorOrgID: 187,
			BranchID: &branch, VariantBranchID: &branch,
			PriceMinor: 3000, MinOrderQty: 1, StockQty: 50, // cheaper, but quota blocked
			VendorActive: true, ProductActive: true,
		},
		{
			ProductID: 10, VariantID: 902, VendorOrgID: 188,
			BranchID: &branch, VariantBranchID: &branch,
			PriceMinor: 4000, MinOrderQty: 1, StockQty: 50, // more expensive, but allowed
			VendorActive: true, ProductActive: true,
		},
	}

	candidates, err := s.buildCandidates(context.Background(), line, offers,
		map[int64]coverageVerdict{}, map[instKey]bool{})
	if err != nil {
		t.Fatalf("buildCandidates: %v", err)
	}

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].Eligible {
		t.Errorf("expected candidate 0 (901) to be ineligible due to quota")
	}
	if !candidates[1].Eligible {
		t.Errorf("expected candidate 1 (902) to be eligible")
	}

	outcome, _ := smartorder.OutcomeFor(true, line.EffectiveQty, candidates)
	if outcome != smartorder.OutcomeOrdered {
		t.Fatalf("expected OutcomeOrdered since candidate 902 is eligible, got %q", outcome)
	}
}
