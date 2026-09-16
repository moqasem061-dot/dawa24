package commerce

import (
	"context"
	"testing"
	"time"
)

func TestDispositionMapping(t *testing.T) {
	cases := []struct {
		reason          Reason
		allowed         bool
		wantDisposition Disposition
		wantOrderable   bool
		wantBlocked     bool
		wantHidden      bool
	}{
		{
			reason:          ReasonOK,
			allowed:         true,
			wantDisposition: DispositionOrderable,
			wantOrderable:   true,
		},
		{
			reason:          ReasonBelowMinimum,
			allowed:         false,
			wantDisposition: DispositionOrderable,
			wantOrderable:   true,
		},
		{
			reason:          ReasonOutOfStock,
			allowed:         false,
			wantDisposition: DispositionBlocked,
			wantBlocked:     true,
		},
		{
			reason:          ReasonInsufficientStock,
			allowed:         false,
			wantDisposition: DispositionBlocked,
			wantBlocked:     true,
		},
		{
			reason:          ReasonQuotaExhausted,
			allowed:         false,
			wantDisposition: DispositionBlocked,
			wantBlocked:     true,
		},
		{
			reason:          ReasonQuotaExceeded,
			allowed:         false,
			wantDisposition: DispositionBlocked,
			wantBlocked:     true,
		},
		{
			reason:          ReasonNotCovered,
			allowed:         false,
			wantDisposition: DispositionHidden,
			wantHidden:      true,
		},
		{
			reason:          ReasonBranchNoLocation,
			allowed:         false,
			wantDisposition: DispositionHidden,
			wantHidden:      true,
		},
		{
			reason:          ReasonBranchNoInstitutionalWorks,
			allowed:         false,
			wantDisposition: DispositionHidden,
			wantHidden:      true,
		},
		{
			reason:          ReasonBranchInstitutionalMismatch,
			allowed:         false,
			wantDisposition: DispositionHidden,
			wantHidden:      true,
		},
		{
			reason:          ReasonOwnOrganization,
			allowed:         false,
			wantDisposition: DispositionHidden,
			wantHidden:      true,
		},
		{
			reason:          ReasonVendorInvalid,
			allowed:         false,
			wantDisposition: DispositionHidden,
			wantHidden:      true,
		},
		{
			reason:          ReasonVendorUnapproved,
			allowed:         false,
			wantDisposition: DispositionHidden,
			wantHidden:      true,
		},
		{
			reason:          ReasonWrongVendor,
			allowed:         false,
			wantDisposition: DispositionHidden,
			wantHidden:      true,
		},
		{
			reason:          ReasonVariantInvalid,
			allowed:         false,
			wantDisposition: DispositionHidden,
			wantHidden:      true,
		},
		{
			reason:          ReasonVariantInactive,
			allowed:         false,
			wantDisposition: DispositionHidden,
			wantHidden:      true,
		},
		{
			reason:          ReasonBranchInvalid,
			allowed:         false,
			wantDisposition: DispositionHidden,
			wantHidden:      true,
		},
		{
			reason:          ReasonBranchNotOwned,
			allowed:         false,
			wantDisposition: DispositionHidden,
			wantHidden:      true,
		},
		{
			reason:          ReasonQuantityInvalid,
			allowed:         false,
			wantDisposition: DispositionHidden,
			wantHidden:      true,
		},
		{
			reason:          Reason("unexpected_error"),
			allowed:         false,
			wantDisposition: DispositionHidden,
			wantHidden:      true,
		},
	}

	for _, tc := range cases {
		t.Run(string(tc.reason), func(t *testing.T) {
			res := AvailabilityResult{
				Allowed: tc.allowed,
				Reason:  tc.reason,
			}
			got := res.Disposition()
			if got != tc.wantDisposition {
				t.Errorf("Disposition() = %v, want %v", got, tc.wantDisposition)
			}
			if res.IsOrderable() != tc.wantOrderable {
				t.Errorf("IsOrderable() = %v, want %v", res.IsOrderable(), tc.wantOrderable)
			}
			if res.IsBlocked() != tc.wantBlocked {
				t.Errorf("IsBlocked() = %v, want %v", res.IsBlocked(), tc.wantBlocked)
			}
			if res.IsHidden() != tc.wantHidden {
				t.Errorf("IsHidden() = %v, want %v", res.IsHidden(), tc.wantHidden)
			}
		})
	}
}

func TestCheckAvailabilityBatchMatchesCheckAvailability(t *testing.T) {
	p := healthyProbe()
	svc := serviceWith(p)
	req := healthyRequest()

	single, err := svc.CheckAvailability(context.Background(), req)
	if err != nil {
		t.Fatalf("CheckAvailability: %v", err)
	}

	batch, err := svc.CheckAvailabilityBatch(context.Background(), req.CustomerOrgID, req.CustomerBranchID, req.When, []AvailabilityLine{
		{VariantID: req.VariantID, VendorOrgID: req.VendorOrgID, Quantity: req.Quantity},
	})
	if err != nil {
		t.Fatalf("CheckAvailabilityBatch: %v", err)
	}

	batchRes, ok := batch[req.VariantID]
	if !ok {
		t.Fatalf("expected variant %d in batch result", req.VariantID)
	}

	if single.Allowed != batchRes.Allowed || single.Reason != batchRes.Reason || single.MaxQuantity != batchRes.MaxQuantity {
		t.Errorf("mismatch: single=%+v, batch=%+v", single, batchRes)
	}
}

func TestCheckAvailabilityBatch_MultiLine(t *testing.T) {
	lat, lon := 30.0444, 31.2357
	p := &stubProbe{
		variant:       VariantAvailability{ID: 10, OrganizationID: 7, StockQty: 10, Active: true},
		vendor:        VendorAvailability{ID: 7, IsVendor: true, Approved: true},
		branch:        BranchAvailability{ID: 3, OrganizationID: 99, Latitude: &lat, Longitude: &lon, InstitutionalWorks: []string{"retail"}},
		covers:        true,
		instConnected: true,
	}
	svc := serviceWith(p)

	lines := []AvailabilityLine{
		{VariantID: 10, VendorOrgID: 7, Quantity: 2},  // Allowed
		{VariantID: 11, VendorOrgID: 7, Quantity: 0},  // QuantityInvalid
		{VariantID: 12, VendorOrgID: 99, Quantity: 2}, // OwnOrganization
	}

	batch, err := svc.CheckAvailabilityBatch(context.Background(), 99, 3, time.Now(), lines)
	if err != nil {
		t.Fatalf("CheckAvailabilityBatch: %v", err)
	}

	if len(batch) != 3 {
		t.Fatalf("expected 3 results, got %d", len(batch))
	}

	if !batch[10].Allowed || batch[10].Reason != ReasonOK {
		t.Errorf("line 10: expected allowed, got %+v", batch[10])
	}
	if batch[11].Allowed || batch[11].Reason != ReasonQuantityInvalid {
		t.Errorf("line 11: expected quantity_invalid, got %+v", batch[11])
	}
	if batch[12].Allowed || batch[12].Reason != ReasonOwnOrganization {
		t.Errorf("line 12: expected own_organization, got %+v", batch[12])
	}
}
