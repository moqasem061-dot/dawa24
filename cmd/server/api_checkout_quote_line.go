package main

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

type resolvedAPIQuoteLine struct {
	variant *catalog.ProductVariant
	offer   *promo.OfferProductWithOffer
	line    commerce.CheckoutLineItem
}

func (q *apiQuoteResolver) quoteLine(ctx context.Context, request commerce.CheckoutInput, submitted commerce.CheckoutLineItem) (resolvedAPIQuoteLine, error) {
	if submitted.OfferProductID != nil {
		if *submitted.OfferProductID < 0 {
			return resolvedAPIQuoteLine{}, quoteUnavailable()
		}
		if *submitted.OfferProductID == 0 {
			submitted.OfferProductID = nil
		}
	}
	variant, product, err := q.quoteProduct(ctx, submitted)
	if err != nil {
		return resolvedAPIQuoteLine{}, err
	}
	unitPrice := variant.EffectiveSellingPrice()
	var offer *promo.OfferProductWithOffer
	if request.OfferID > 0 || submitted.OfferProductID != nil {
		offer, err = q.offerForLine(ctx, request, submitted, variant)
		if err != nil {
			return resolvedAPIQuoteLine{}, err
		}
		unitPrice, _ = promo.EffectivePrice(variant.Price, offer.Product, offer.Offer)
	}
	if !unitPrice.IsPositive() {
		return resolvedAPIQuoteLine{}, quoteUnavailable()
	}
	discount := money.Zero
	if variant.Price.Minor() > unitPrice.Minor() {
		discount, err = variant.Price.Sub(unitPrice)
		if err != nil {
			return resolvedAPIQuoteLine{}, err
		}
		discount, err = discount.MulInt(int64(submitted.Quantity))
		if err != nil {
			return resolvedAPIQuoteLine{}, err
		}
	}
	line := commerce.CheckoutLineItem{VendorOrgID: variant.OrganizationID, ProductID: &variant.ProductID, ProductVariantID: &variant.ID, ProductName: product.Name, VariantName: variant.Name, SKU: variant.SKU, Quantity: submitted.Quantity, UnitPrice: unitPrice, ListPrice: variant.Price, OriginalPrice: variant.Price, OriginalDiscount: variant.Discount, DiscountAmount: discount, CostPrice: variant.CostPrice, CostDiscountPercentage: variant.CostDiscountPercentage}
	if offer != nil {
		line.OfferProductID = &offer.Product.ID
	}
	return resolvedAPIQuoteLine{variant: variant, offer: offer, line: line}, nil
}

func (q *apiQuoteResolver) quoteProduct(ctx context.Context, submitted commerce.CheckoutLineItem) (*catalog.ProductVariant, *catalog.Product, error) {
	if submitted.ProductVariantID == nil || *submitted.ProductVariantID <= 0 || submitted.Quantity <= 0 || submitted.Quantity > 100000 {
		return nil, nil, quoteUnavailable()
	}
	sys := database.AsSystem(ctx)
	variant, err := q.catalog.GetVariant(sys, *submitted.ProductVariantID)
	if err != nil {
		return nil, nil, err
	}
	if variant == nil || variant.DeletedAt != nil || variant.Status != catalog.StatusActive || variant.OrganizationID <= 0 || (submitted.VendorOrgID > 0 && submitted.VendorOrgID != variant.OrganizationID) || (submitted.ProductID != nil && (*submitted.ProductID < 0 || (*submitted.ProductID > 0 && *submitted.ProductID != variant.ProductID))) {
		return nil, nil, quoteUnavailable()
	}
	if variant.BranchID != nil && *variant.BranchID > 0 {
		branch, err := q.org.GetBranch(sys, *variant.BranchID)
		if err != nil {
			return nil, nil, err
		}
		if branch == nil || branch.OrganizationID != variant.OrganizationID || branch.Status == "inactive" || branch.Status == "suspended" {
			return nil, nil, quoteUnavailable()
		}
	}
	product, _, err := q.catalog.GetProduct(sys, variant.ProductID)
	if err != nil {
		return nil, nil, err
	}
	if product == nil || (product.Status != catalog.StatusActive && product.Status != catalog.StatusPending) {
		return nil, nil, quoteUnavailable()
	}
	return variant, product, nil
}
