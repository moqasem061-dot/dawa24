package commerce

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

// stubProbe answers the cross-module questions with fixtures. Each field can be
// overridden per test; the zero value is a healthy in-stock, covered line.
type stubProbe struct {
	variant       VariantAvailability
	vendor        VendorAvailability
	branch        BranchAvailability
	covers        bool
	instConnected bool
	variantErr    error
	vendorErr     error
	branchErr     error
	coverErr      error
	instErr       error
}

func (p *stubProbe) Variant(context.Context, int64) (VariantAvailability, error) {
	return p.variant, p.variantErr
}
func (p *stubProbe) Vendor(context.Context, int64) (VendorAvailability, error) {
	return p.vendor, p.vendorErr
}
func (p *stubProbe) CustomerBranch(context.Context, int64) (BranchAvailability, error) {
	return p.branch, p.branchErr
}
func (p *stubProbe) VendorCovers(context.Context, int64, int64, float64, float64, time.Weekday, *int64) (bool, error) {
	return p.covers, p.coverErr
}
func (p *stubProbe) VendorInstitutionalConnection(context.Context, int64, int64, int64) (bool, error) {
	return p.instConnected, p.instErr
}
func (p *stubProbe) VariantsByIDs(ctx context.Context, ids []int64) (map[int64]VariantAvailability, error) {
	if p.variantErr != nil {
		return nil, p.variantErr
	}
	out := make(map[int64]VariantAvailability, len(ids))
	for _, id := range ids {
		if p.variant.ID == id || p.variant.ID == 0 {
			v := p.variant
			v.ID = id
			out[id] = v
		} else {
			out[id] = p.variant
		}
	}
	return out, nil
}
func (p *stubProbe) VendorsByIDs(ctx context.Context, ids []int64) (map[int64]VendorAvailability, error) {
	if p.vendorErr != nil {
		return nil, p.vendorErr
	}
	out := make(map[int64]VendorAvailability, len(ids))
	for _, id := range ids {
		if p.vendor.ID == id || p.vendor.ID == 0 {
			v := p.vendor
			v.ID = id
			out[id] = v
		} else {
			out[id] = p.vendor
		}
	}
	return out, nil
}
func (p *stubProbe) VendorInstitutionalConnections(ctx context.Context, customerBranchID int64, lines []AvailabilityLine) (map[int64]bool, error) {
	if p.instErr != nil {
		return nil, p.instErr
	}
	out := make(map[int64]bool, len(lines))
	for _, l := range lines {
		out[l.VariantID] = p.instConnected
	}
	return out, nil
}

func healthyProbe() *stubProbe {
	lat, lon := 30.0444, 31.2357
	return &stubProbe{
		variant:       VariantAvailability{ID: 10, OrganizationID: 7, StockQty: 5, Active: true},
		vendor:        VendorAvailability{ID: 7, IsVendor: true, Approved: true},
		branch:        BranchAvailability{ID: 3, OrganizationID: 99, Latitude: &lat, Longitude: &lon, InstitutionalWorks: []string{"retail"}},
		covers:        true,
		instConnected: true,
	}
}

func healthyRequest() AvailabilityRequest {
	return AvailabilityRequest{
		VariantID:        10,
		VendorOrgID:      7,
		CustomerOrgID:    99,
		CustomerBranchID: 3,
		Quantity:         2,
		When:             time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
	}
}

func serviceWith(p AvailabilityProbe) *Service {
	s := NewService(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if p != nil {
		s.SetAvailabilityProbe(p)
	}
	return s
}

func TestCheckAvailability(t *testing.T) {
	cases := []struct {
		name       string
		probe      func(*stubProbe)
		request    func(*AvailabilityRequest)
		wantAllow  bool
		wantReason Reason
		wantMax    int
	}{
		{
			name:      "a healthy in-stock covered line is allowed",
			wantAllow: true,
			wantMax:   5,
		},
		{
			// The marketplace invariant Smart Ordering has always enforced as
			// ReasonOwnOrg, and the ordinary purchase path did not: it became
			// reachable when suppliers gained the buying surface and began
			// browsing a catalogue their own variants are listed in.
			name: "a company buying from itself is refused",
			request: func(r *AvailabilityRequest) {
				r.CustomerOrgID = r.VendorOrgID
			},
			wantReason: ReasonOwnOrganization,
		},
		{
			// The refusal must not depend on anything the probe knows, so a
			// self-supplied line is refused even when every other condition is
			// broken too. Otherwise the first failure reported would vary with
			// the state of the catalogue.
			name: "self-supply is refused before stock, coverage or approval",
			probe: func(p *stubProbe) {
				p.vendor.Approved = false
				p.variant.StockQty = 0
				p.covers = false
			},
			request: func(r *AvailabilityRequest) {
				r.CustomerOrgID = r.VendorOrgID
			},
			wantReason: ReasonOwnOrganization,
		},
		{
			// A signed-out or org-less caller has no company to supply itself
			// from; the check must not fire on a zero.
			name: "a caller with no organization is not treated as self-supplying",
			request: func(r *AvailabilityRequest) {
				r.CustomerOrgID = 0
			},
			wantAllow: true,
			wantMax:   5,
		},
		{
			// The old code did `if vendorOrgID <= 0 { vendorOrgID = 1 }`, which
			// silently attributed the line to whichever org has id 1.
			name:       "a missing vendor is refused, never defaulted",
			request:    func(r *AvailabilityRequest) { r.VendorOrgID = 0 },
			wantReason: ReasonVendorInvalid,
		},
		{
			name:       "an unapproved vendor is refused",
			probe:      func(p *stubProbe) { p.vendor.Approved = false },
			wantReason: ReasonVendorUnapproved,
		},
		{
			name:       "a customer organization posing as a vendor is refused",
			probe:      func(p *stubProbe) { p.vendor.IsVendor = false },
			wantReason: ReasonVendorInvalid,
		},
		{
			name:       "an inactive variant is refused",
			probe:      func(p *stubProbe) { p.variant.Active = false },
			wantReason: ReasonVariantInactive,
		},
		{
			// Guards against a crafted form that pairs a cheap variant with a
			// different supplier.
			name:       "a variant belonging to another supplier is refused",
			probe:      func(p *stubProbe) { p.variant.OrganizationID = 12345 },
			wantReason: ReasonWrongVendor,
		},
		{
			// The old check was `if variant.StockQty > 0 && qty > variant.StockQty`,
			// so a zero-stock variant skipped validation entirely.
			name:       "zero stock is refused rather than skipped",
			probe:      func(p *stubProbe) { p.variant.StockQty = 0 },
			wantReason: ReasonOutOfStock,
		},
		{
			// The old code silently set qty = StockQty and told the pharmacy
			// nothing.
			name:       "over-stock is refused and reports what is available",
			request:    func(r *AvailabilityRequest) { r.Quantity = 100 },
			wantReason: ReasonInsufficientStock,
			wantMax:    5,
		},
		{
			name:       "below the supplier's minimum order quantity is refused",
			probe:      func(p *stubProbe) { p.variant.MinOrderQty = 3 },
			request:    func(r *AvailabilityRequest) { r.Quantity = 2 },
			wantReason: ReasonBelowMinimum,
		},
		{
			name:       "zero quantity is refused",
			request:    func(r *AvailabilityRequest) { r.Quantity = 0 },
			wantReason: ReasonQuantityInvalid,
		},
		{
			name:       "a negative quantity is refused",
			request:    func(r *AvailabilityRequest) { r.Quantity = -5 },
			wantReason: ReasonQuantityInvalid,
		},
		{
			name:       "no receiving branch selected is refused",
			request:    func(r *AvailabilityRequest) { r.CustomerBranchID = 0 },
			wantReason: ReasonBranchInvalid,
		},
		{
			name:       "another organization's branch is refused",
			probe:      func(p *stubProbe) { p.branch.OrganizationID = 4242 },
			wantReason: ReasonBranchNotOwned,
		},
		{
			name:       "a branch with no map location and no city cannot be covered",
			probe:      func(p *stubProbe) { p.branch.Latitude, p.branch.Longitude = nil, nil },
			wantReason: ReasonBranchNoLocation,
		},
		{
			name: "a branch with city_id but no coordinates is covered when vendor covers it",
			probe: func(p *stubProbe) {
				p.branch.Latitude, p.branch.Longitude = nil, nil
				cID := int64(45)
				p.branch.CityID = &cID
				p.covers = true
			},
			wantAllow: true,
			wantMax:   5,
		},
		{
			// Nothing checked this before, so a pharmacy could order from a
			// supplier that does not deliver to it at all.
			name:       "a supplier that does not cover the branch is refused",
			probe:      func(p *stubProbe) { p.covers = false },
			wantReason: ReasonNotCovered,
		},
		{
			name: "a branch with zero institutional works is refused",
			probe: func(p *stubProbe) {
				p.branch.InstitutionalWorks = nil
			},
			wantReason: ReasonBranchNoInstitutionalWorks,
			wantMax:    5,
		},
		{
			name: "a branch whose institutional works are not connected to vendor is refused",
			probe: func(p *stubProbe) {
				p.instConnected = false
			},
			wantReason: ReasonBranchInstitutionalMismatch,
			wantMax:    5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := healthyProbe()
			if tc.probe != nil {
				tc.probe(p)
			}
			req := healthyRequest()
			if tc.request != nil {
				tc.request(&req)
			}

			got, err := serviceWith(p).CheckAvailability(context.Background(), req)
			if err != nil {
				t.Fatalf("CheckAvailability returned an error: %v", err)
			}
			if got.Allowed != tc.wantAllow {
				t.Fatalf("Allowed = %v (reason %q), want %v", got.Allowed, got.Reason, tc.wantAllow)
			}
			if got.Reason != tc.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, tc.wantReason)
			}
			if tc.wantMax != 0 && got.MaxQuantity != tc.wantMax {
				t.Errorf("MaxQuantity = %d, want %d", got.MaxQuantity, tc.wantMax)
			}
			if !got.Allowed && got.MessageAr == "" {
				t.Error("a refusal must carry an Arabic message the pharmacy can read")
			}
		})
	}
}

// A probe failure must surface as an error, never as a quiet allow. The old
// handler swallowed the stock lookup's error with `err == nil`, so a failing
// query meant no stock check ran at all.
func TestCheckAvailability_ProbeErrorsSurface(t *testing.T) {
	boom := errors.New("database unavailable")

	for _, tc := range []struct {
		name  string
		probe func(*stubProbe)
	}{
		{"vendor lookup fails", func(p *stubProbe) { p.vendorErr = boom }},
		{"variant lookup fails", func(p *stubProbe) { p.variantErr = boom }},
		{"branch lookup fails", func(p *stubProbe) { p.branchErr = boom }},
		{"coverage lookup fails", func(p *stubProbe) { p.coverErr = boom }},
		{"institutional connection lookup fails", func(p *stubProbe) { p.instErr = boom }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := healthyProbe()
			tc.probe(p)

			got, err := serviceWith(p).CheckAvailability(context.Background(), healthyRequest())
			if err == nil {
				t.Fatalf("expected an error, got result %+v", got)
			}
			if got.Allowed {
				t.Fatal("a failed probe must never report the line as allowed")
			}
		})
	}
}

// Without a probe the service cannot prove anything, so it must fail closed.
func TestCheckAvailability_FailsClosedWithoutProbe(t *testing.T) {
	got, err := serviceWith(nil).CheckAvailability(context.Background(), healthyRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Allowed {
		t.Fatal("with no availability probe the service must refuse, not allow")
	}
}
