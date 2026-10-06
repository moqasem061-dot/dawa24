package ui

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func TestSelectedPromotionalPriceMatchesSupplyAndOffer(t *testing.T) {
	offerID, variantID, customPrice := int64(55), int64(200), money.MustParse("60.00")
	line := &commerce.CartItem{ProductID: 100, ProductVariantID: variantID, OrganizationID: 300, OfferID: &offerID}
	candidates := []*promo.OfferProductWithOffer{{
		Product: &promo.OfferProduct{OfferID: offerID, ProductID: 100, VariantID: &variantID, CustomPrice: &customPrice},
		Offer:   &promo.Offer{ID: offerID, OrganizationID: 300},
	}}
	price, err := selectedPromotionalPrice(candidates, line, money.MustParse("100.00"))
	if err != nil || price.String() != "60.00" {
		t.Fatalf("selected quote = %s, %v; want 60.00", price, err)
	}

	wrongSupplier := *line
	wrongSupplier.OrganizationID = 301
	if _, err := selectedPromotionalPrice(candidates, &wrongSupplier, money.MustParse("100.00")); err == nil {
		t.Fatal("promotion from another supplier was accepted")
	}
	wrongVariant := *line
	wrongVariant.ProductVariantID = 201
	if _, err := selectedPromotionalPrice(candidates, &wrongVariant, money.MustParse("100.00")); err == nil {
		t.Fatal("promotion for another variant was accepted")
	}
}
