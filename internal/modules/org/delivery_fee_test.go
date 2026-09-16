package org

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestCalculateDeliveryFee(t *testing.T) {
	repo := newMockOrgRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)
	ctx := context.Background()

	vendorOrgID := int64(10)

	// 1. When no bands configured -> returns zero, false
	fee, matched, err := svc.CalculateDeliveryFee(ctx, vendorOrgID, 3000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched || fee.IsPositive() {
		t.Errorf("expected no match and zero fee, got matched=%t fee=%s", matched, fee.String())
	}

	// 2. Configure 3 distance bands in meters:
	// Band 1: 0 - 5,000m (0-5km) -> 20.00 EGP
	// Band 2: 5,001 - 15,000m (5-15km) -> 40.00 EGP
	// Band 3: 15,001 - 30,000m (15-30km) -> 75.00 EGP
	bands := []*DeliveryBand{
		{ID: 1, OrganizationID: vendorOrgID, FromMeters: 0, ToMeters: 5000, Fee: money.FromMinor(2000), IsActive: true},
		{ID: 2, OrganizationID: vendorOrgID, FromMeters: 5001, ToMeters: 15000, Fee: money.FromMinor(4000), IsActive: true},
		{ID: 3, OrganizationID: vendorOrgID, FromMeters: 15001, ToMeters: 30000, Fee: money.FromMinor(7500), IsActive: true},
	}
	if err := svc.SaveDeliveryBands(ctx, vendorOrgID, bands); err != nil {
		t.Fatalf("failed to save delivery bands: %v", err)
	}

	// Test 3,000 meters -> falls into Band 1 (20.00 EGP)
	fee, matched, err = svc.CalculateDeliveryFee(ctx, vendorOrgID, 3000)
	if err != nil || !matched || fee.Minor() != 2000 {
		t.Errorf("expected 20.00 EGP for 3000m, got fee=%s matched=%t err=%v", fee.String(), matched, err)
	}

	// Test 10,000 meters -> falls into Band 2 (40.00 EGP)
	fee, matched, err = svc.CalculateDeliveryFee(ctx, vendorOrgID, 10000)
	if err != nil || !matched || fee.Minor() != 4000 {
		t.Errorf("expected 40.00 EGP for 10000m, got fee=%s matched=%t err=%v", fee.String(), matched, err)
	}

	// Test 25,000 meters -> falls into Band 3 (75.00 EGP)
	fee, matched, err = svc.CalculateDeliveryFee(ctx, vendorOrgID, 25000)
	if err != nil || !matched || fee.Minor() != 7500 {
		t.Errorf("expected 75.00 EGP for 25000m, got fee=%s matched=%t err=%v", fee.String(), matched, err)
	}

	// Test 45,000 meters (beyond max band) -> capped/matches highest tier (75.00 EGP)
	fee, matched, err = svc.CalculateDeliveryFee(ctx, vendorOrgID, 45000)
	if err != nil || !matched || fee.Minor() != 7500 {
		t.Errorf("expected 75.00 EGP for 45000m, got fee=%s matched=%t err=%v", fee.String(), matched, err)
	}
}
