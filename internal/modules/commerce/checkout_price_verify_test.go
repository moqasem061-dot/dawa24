package commerce

import (
	"context"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type stubPriceAvailabilityProbe struct {
	variants map[int64]VariantAvailability
}

func (p *stubPriceAvailabilityProbe) Variant(ctx context.Context, variantID int64) (VariantAvailability, error) {
	if v, ok := p.variants[variantID]; ok {
		return v, nil
	}
	return VariantAvailability{}, nil
}

func (p *stubPriceAvailabilityProbe) Vendor(ctx context.Context, orgID int64) (VendorAvailability, error) {
	return VendorAvailability{ID: orgID, IsVendor: true, Approved: true}, nil
}

func (p *stubPriceAvailabilityProbe) CustomerBranch(ctx context.Context, branchID int64) (BranchAvailability, error) {
	return BranchAvailability{ID: branchID}, nil
}

func (p *stubPriceAvailabilityProbe) VendorCovers(ctx context.Context, vendorOrgID, vendorBranchID int64, lat, lon float64, day time.Weekday, cityID *int64) (bool, error) {
	return true, nil
}

func (p *stubPriceAvailabilityProbe) VendorInstitutionalConnection(ctx context.Context, vendorOrgID int64, customerBranchID int64, variantID int64) (bool, error) {
	return true, nil
}

func (p *stubPriceAvailabilityProbe) VariantsByIDs(ctx context.Context, variantIDs []int64) (map[int64]VariantAvailability, error) {
	out := make(map[int64]VariantAvailability)
	for _, id := range variantIDs {
		if v, ok := p.variants[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

func (p *stubPriceAvailabilityProbe) VendorsByIDs(ctx context.Context, orgIDs []int64) (map[int64]VendorAvailability, error) {
	out := make(map[int64]VendorAvailability)
	for _, id := range orgIDs {
		out[id] = VendorAvailability{ID: id, IsVendor: true, Approved: true}
	}
	return out, nil
}

func (p *stubPriceAvailabilityProbe) VendorInstitutionalConnections(ctx context.Context, customerBranchID int64, lines []AvailabilityLine) (map[int64]bool, error) {
	out := make(map[int64]bool)
	for _, l := range lines {
		out[l.VariantID] = true
	}
	return out, nil
}

func TestCheckoutPriceTamperingEnforcement(t *testing.T) {
	ctx := context.Background()
	authPrice, _ := money.Parse("150.00")
	authDiscount, _ := money.Parse("10.00") // 10%
	// 150 * (1 - 0.10) = 135.00
	effPrice, _ := money.Parse("135.00")

	probe := &stubPriceAvailabilityProbe{
		variants: map[int64]VariantAvailability{
			42: {
				ID:             42,
				OrganizationID: 10,
				Active:         true,
				Price:          authPrice,
				Discount:       authDiscount,
				EffectivePrice: effPrice,
				IsNegotiable:   false,
			},
		},
	}

	svc := NewService(nil, nil)
	svc.SetAvailabilityProbe(probe)

	tamperedPrice, _ := money.Parse("0.50")
	variantID := int64(42)

	input := CheckoutInput{
		CustomerID: 1,
		Items: []CheckoutLineItem{
			{
				ProductVariantID: &variantID,
				VendorOrgID:      10,
				Quantity:         2,
				UnitPrice:        tamperedPrice, // Client attempts to pay 0.50 instead of 135.00
			},
		},
	}

	err := svc.verifyAndSanitizeCheckoutPrices(ctx, &input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify that the server strictly sanitized and overridden the tampered price
	sanitizedItem := input.Items[0]
	if sanitizedItem.UnitPrice.Minor() != effPrice.Minor() {
		t.Fatalf("expected unit price to be sanitized to %v, got %v", effPrice, sanitizedItem.UnitPrice)
	}
	if sanitizedItem.ListPrice.Minor() != authPrice.Minor() {
		t.Fatalf("expected list price to be %v, got %v", authPrice, sanitizedItem.ListPrice)
	}
}

func TestCheckoutNegotiationBlockedWhenNotNegotiable(t *testing.T) {
	ctx := context.Background()
	authPrice, _ := money.Parse("100.00")

	probe := &stubPriceAvailabilityProbe{
		variants: map[int64]VariantAvailability{
			99: {
				ID:             99,
				OrganizationID: 5,
				Active:         true,
				Price:          authPrice,
				EffectivePrice: authPrice,
				IsNegotiable:   false, // Vendor does NOT allow negotiation
			},
		},
	}

	svc := NewService(nil, nil)
	svc.SetAvailabilityProbe(probe)

	proposedPrice, _ := money.Parse("60.00")
	variantID := int64(99)

	input := CheckoutInput{
		CustomerID:    1,
		IsNegotiation: true,
		Items: []CheckoutLineItem{
			{
				ProductVariantID: &variantID,
				VendorOrgID:      5,
				Quantity:         10,
				UnitPrice:        proposedPrice,
			},
		},
	}

	err := svc.verifyAndSanitizeCheckoutPrices(ctx, &input)
	if err == nil {
		t.Fatal("expected negotiation to be rejected for non-negotiable variant, but got nil")
	}
}
