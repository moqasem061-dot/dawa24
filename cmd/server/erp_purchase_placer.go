package main

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	billingPostgres "github.com/moqasem061-dot/dawa24/internal/modules/billing/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	commercePostgres "github.com/moqasem061-dot/dawa24/internal/modules/commerce/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// purchaseCheckout is the part of the commerce service the placer uses: the
// REST checkout, which re-derives every amount and refuses on any difference.
type purchaseCheckout interface {
	CheckoutAPI(ctx context.Context, submitted commerce.CheckoutInput) (*commerce.Order, error)
}

// purchaseVariants resolves a variant public id to its internal row. It is the
// erp repository's E6 read.
type purchaseVariants interface {
	VariantOfferAsSystem(ctx context.Context, publicID string) (erp.VariantOfferRow, error)
}

// erpPurchasePlacer places a purchase order through the website's checkout rules
// (plan.md S7 step 5). It quotes the lines, compares any sent price with the
// quote, and submits the quoted input to CheckoutAPI.
type erpPurchasePlacer struct {
	checkout purchaseCheckout
	quote    commerce.APIQuoteResolver
	variants purchaseVariants
}

var _ erp.PurchasePlacer = (*erpPurchasePlacer)(nil)

// Place implements erp.PurchasePlacer.
func (p *erpPurchasePlacer) Place(ctx context.Context, req erp.PurchaseRequest) (erp.PlacedOrder, error) {
	branch := req.BranchID
	input := commerce.CheckoutInput{
		CustomerID: req.BuyingUserID, CustomerOrgID: req.BuyerOrgID, CustomerOrgType: req.BuyerOrgType,
		BranchID: &branch, Notes: req.Notes,
	}
	for i, line := range req.Lines {
		row, err := p.variants.VariantOfferAsSystem(ctx, line.VariantID)
		if errors.Is(err, erp.ErrNotFound) {
			return erp.PlacedOrder{}, erp.NewError("ERP_PO_ITEM_NOT_FOUND", 422, map[string]any{"line": i + 1, "variantId": line.VariantID})
		}
		if err != nil {
			return erp.PlacedOrder{}, erp.NewError("ERP_INTERNAL", 500, nil)
		}
		variantID := row.ID
		input.Items = append(input.Items, commerce.CheckoutLineItem{VendorOrgID: row.OrganizationID, ProductVariantID: &variantID, Quantity: line.Quantity})
	}
	quoted, err := p.quote(ctx, input)
	if err != nil {
		return erp.PlacedOrder{}, purchaseCheckoutError(err, req.Lines)
	}
	var mismatches []map[string]any
	for i, line := range req.Lines {
		if line.UnitPrice != nil && i < len(quoted.Items) && line.UnitPrice.Minor() != quoted.Items[i].UnitPrice.Minor() {
			mismatches = append(mismatches, map[string]any{"line": i + 1, "sentPrice": line.UnitPrice.String(), "correctPrice": quoted.Items[i].UnitPrice.String()})
		}
	}
	if len(mismatches) > 0 {
		return erp.PlacedOrder{}, erp.NewError("ERP_PO_PRICE_MISMATCH", 409, map[string]any{"lines": mismatches})
	}
	order, err := p.checkout.CheckoutAPI(ctx, quoted)
	if err != nil {
		return erp.PlacedOrder{}, purchaseCheckoutError(err, req.Lines)
	}
	return erp.PlacedOrder{OrderID: order.ID, OrderNumber: order.OrderNumber, Total: order.TotalAmount}, nil
}

// purchaseCheckoutError maps the website's refusal onto the ERP codes. A price
// change reported by the checkout names the correct unit price of each line.
func purchaseCheckoutError(err error, lines []erp.PurchaseLine) error {
	var a *apperr.Error
	if !errors.As(err, &a) {
		return erp.NewError("ERP_INTERNAL", 500, nil)
	}
	switch {
	case a.Code == "checkout.wallet_on_website":
		return erp.NewError("ERP_PO_PAYMENT_NOT_SUPPORTED", 409, nil)
	case a.Code == "checkout.price_changed":
		var mismatches []map[string]any
		for i, line := range lines {
			correct, ok := a.Fields[priceField(i)]
			if !ok || line.UnitPrice == nil {
				continue
			}
			mismatches = append(mismatches, map[string]any{"line": i + 1, "sentPrice": line.UnitPrice.String(), "correctPrice": correct})
		}
		return erp.NewError("ERP_PO_PRICE_MISMATCH", 409, map[string]any{"lines": mismatches})
	case a.Kind == apperr.KindUnavailable:
		return erp.NewError("ERP_UNAVAILABLE", 503, nil)
	}
	return erp.NewError("ERP_PO_REFUSED", 409, map[string]any{"reason": a.Code})
}

func priceField(i int) string {
	return "items." + strconv.Itoa(i) + ".unit_price"
}

// newERPPurchaseCheckout builds the commerce service the placer checks out
// through, wired as the website's API checkout is (routes.go).
func newERPPurchaseCheckout(db *database.DB, log *slog.Logger, docs commerce.RequiredDocsChecker, announce commerce.OrderPlacedHook, probe commerce.AvailabilityProbe, quote commerce.APIQuoteResolver) *commerce.Service {
	repo := commercePostgres.NewRepository(db).WithWalletRefunder(billingPostgres.NewRepository(db)).WithWalletPayer(billingPostgres.NewRepository(db))
	svc := commerce.NewService(repo, log)
	svc.SetRequiredDocsChecker(docs)
	svc.OnOrderPlaced(announce)
	svc.SetAvailabilityProbe(probe)
	svc.SetAPIQuoteResolver(quote)
	return svc
}
