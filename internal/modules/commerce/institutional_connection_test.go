package commerce

import (
	"context"
	"testing"
)

// TestInstitutionalWorks_AllowedConnections verifies that CheckAvailability
// allows purchases ONLY when the vendor's branch institutional work is connected
// to the pharmacy's branch institutional work as defined by platform settings.
func TestInstitutionalWorks_AllowedConnections(t *testing.T) {
	ctx := context.Background()

	t.Run("allowed when vendor institutional work is connected to pharmacy branch", func(t *testing.T) {
		p := healthyProbe()
		p.branch.InstitutionalWorks = []string{"2"} // Pharmacy (صيدلية)
		p.instConnected = true                      // Connected to vendor's warehouse

		req := healthyRequest()
		res, err := serviceWith(p).CheckAvailability(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Allowed {
			t.Fatalf("expected allowed = true, got false with reason %s (%s)", res.Reason, res.MessageAr)
		}
	})

	t.Run("refused when customer branch has no institutional works", func(t *testing.T) {
		p := healthyProbe()
		p.branch.InstitutionalWorks = nil

		req := healthyRequest()
		res, err := serviceWith(p).CheckAvailability(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Allowed {
			t.Fatalf("expected allowed = false, got true")
		}
		if res.Reason != ReasonBranchNoInstitutionalWorks {
			t.Fatalf("expected reason %s, got %s", ReasonBranchNoInstitutionalWorks, res.Reason)
		}
	})

	t.Run("refused with ReasonBranchInstitutionalMismatch when vendor work is not in pharmacy connections", func(t *testing.T) {
		p := healthyProbe()
		p.branch.InstitutionalWorks = []string{"2"} // Pharmacy (صيدلية)
		p.instConnected = false                     // Vendor branch is NOT in allowed connections of this pharmacy

		req := healthyRequest()
		res, err := serviceWith(p).CheckAvailability(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Allowed {
			t.Fatalf("expected allowed = false, got true")
		}
		if res.Reason != ReasonBranchInstitutionalMismatch {
			t.Fatalf("expected reason %s, got %s", ReasonBranchInstitutionalMismatch, res.Reason)
		}
		if res.MessageAr == "" {
			t.Fatalf("expected Arabic refusal message, got empty string")
		}
	})
}
