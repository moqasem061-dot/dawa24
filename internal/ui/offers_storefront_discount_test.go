package ui

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestOffersForProduct_PreservesVariantDiscountAgainstZeroPromo(t *testing.T) {
	h := &UIHandler{}
	branchID := int64(100)
	buyerActor := authctx.Actor{
		UserID:         1,
		OrganizationID: 10,
		Role:           "customer_admin",
		Scope:          "pharmacy",
		BranchID:       &branchID,
	}
	ctx := authctx.WithActor(context.Background(), buyerActor)
	ctx = authctx.WithBuyingBranch(ctx, authctx.BuyingBranch{Active: &branchID})

	product := &catalog.Product{
		ID:    327363,
		Price: money.FromMinor(5300), // 53.00 EGP
	}

	variantID := int64(41315)
	vendorOrgID := int64(187)

	// Variant has 37% discount
	variant := &catalog.ProductVariant{
		ID:             variantID,
		ProductID:      product.ID,
		OrganizationID: vendorOrgID,
		Price:          money.FromMinor(5300),
		Discount:       money.FromMinor(3700), // 37.00%
		StockQty:       10,
	}

	// Promo module has an offer from same vendor with 0% discount and list price 53.00
	customPrice := money.FromMinor(5300)
	promoOffer := &promo.OfferProductWithOffer{
		Product: &promo.OfferProduct{
			ID:          1,
			OfferID:     12,
			ProductID:   product.ID,
			CustomPrice: &customPrice,
		},
		Offer: &promo.Offer{
			ID:             12,
			OrganizationID: vendorOrgID,
			DiscountType:   promo.DiscountPercentage,
			DiscountValue:  money.Zero,
		},
	}

	env := &offerEnv{
		orgs: map[int64]*org.Organization{
			vendorOrgID: {ID: vendorOrgID, Status: org.StatusApproved},
		},
		branches: make(map[int64]*org.Branch),
		stock: map[int64]int{
			variantID: 10,
		},
		promo: map[int64][]*promo.OfferProductWithOffer{
			product.ID: {promoOffer},
		},
	}

	offers := h.offersForProduct(ctx, product, []*catalog.ProductVariant{variant}, env, "ar")

	if len(offers) != 1 {
		t.Fatalf("expected 1 offer, got %d", len(offers))
	}

	off := offers[0]

	// Price must reflect the 37% discount: 53.00 - 19.61 = 33.39 EGP (3339 minor)
	expectedMinor := int64(3339)
	if off.Price.Minor() != expectedMinor {
		t.Errorf("expected discounted price minor %d (33.39 EGP), got %d (%s)",
			expectedMinor, off.Price.Minor(), off.Price.String())
	}

	if off.DiscountBPS != 3700 {
		t.Errorf("expected DiscountBPS=3700 (37%%), got %d", off.DiscountBPS)
	}

	if !off.DiscountAmount.IsPositive() {
		t.Errorf("expected positive DiscountAmount, got %s", off.DiscountAmount.String())
	}
}

func TestOffersForProduct_AppliesBetterPromoDiscount(t *testing.T) {
	h := &UIHandler{}
	branchID := int64(100)
	buyerActor := authctx.Actor{
		UserID:         1,
		OrganizationID: 10,
		Role:           "customer_admin",
		Scope:          "pharmacy",
		BranchID:       &branchID,
	}
	ctx := authctx.WithActor(context.Background(), buyerActor)
	ctx = authctx.WithBuyingBranch(ctx, authctx.BuyingBranch{Active: &branchID})

	product := &catalog.Product{
		ID:    100,
		Price: money.FromMinor(10000), // 100.00 EGP
	}

	variantID := int64(200)
	vendorOrgID := int64(300)

	// Variant has 10% discount
	variant := &catalog.ProductVariant{
		ID:             variantID,
		ProductID:      product.ID,
		OrganizationID: vendorOrgID,
		Price:          money.FromMinor(10000),
		Discount:       money.FromMinor(1000), // 10.00%
		StockQty:       10,
	}

	// Promo offer offers 40% discount
	promoDiscountPct := 40.0
	promoOffer := &promo.OfferProductWithOffer{
		Product: &promo.OfferProduct{
			ID:                    1,
			OfferID:               55,
			ProductID:             product.ID,
			CustomDiscountPercent: &promoDiscountPct,
		},
		Offer: &promo.Offer{
			ID:             55,
			OrganizationID: vendorOrgID,
		},
	}

	env := &offerEnv{
		orgs: map[int64]*org.Organization{
			vendorOrgID: {ID: vendorOrgID, Status: org.StatusApproved},
		},
		branches: make(map[int64]*org.Branch),
		stock: map[int64]int{
			variantID: 10,
		},
		promo: map[int64][]*promo.OfferProductWithOffer{
			product.ID: {promoOffer},
		},
	}

	offers := h.offersForProduct(ctx, product, []*catalog.ProductVariant{variant}, env, "ar")

	if len(offers) != 1 {
		t.Fatalf("expected 1 offer, got %d", len(offers))
	}

	off := offers[0]
	// Should take the better 40% discount -> 60.00 EGP
	if off.Price.Minor() != 6000 {
		t.Errorf("expected price minor 6000 (60.00 EGP), got %d (%s)", off.Price.Minor(), off.Price.String())
	}
	if off.DiscountBPS != 4000 {
		t.Errorf("expected DiscountBPS=4000 (40%%), got %d", off.DiscountBPS)
	}
}
