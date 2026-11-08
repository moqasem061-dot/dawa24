package main

import (
	"context"
	"errors"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// fakeVariants resolves public ids the way the erp repository does.
type fakeVariants struct {
	rows map[string]erp.VariantOfferRow
}

func (f fakeVariants) VariantOfferAsSystem(_ context.Context, publicID string) (erp.VariantOfferRow, error) {
	row, ok := f.rows[publicID]
	if !ok {
		return erp.VariantOfferRow{}, erp.ErrNotFound
	}
	return row, nil
}

// fakeCheckout records what the placer submits and answers with a fixed result.
type fakeCheckout struct {
	got   []commerce.CheckoutInput
	order *commerce.Order
	err   error
}

func (f *fakeCheckout) CheckoutAPI(_ context.Context, in commerce.CheckoutInput) (*commerce.Order, error) {
	f.got = append(f.got, in)
	return f.order, f.err
}

const testVariantUUID = "3b9c1a4e-6f02-4d58-8c7e-12a4d9e05b33"

// quoteAt is the website quote: every line priced at the given unit price.
func quoteAt(unit string) commerce.APIQuoteResolver {
	return func(_ context.Context, in commerce.CheckoutInput) (commerce.CheckoutInput, error) {
		for i := range in.Items {
			in.Items[i].UnitPrice = money.MustParse(unit)
			in.Items[i].DiscountAmount = money.Zero
		}
		return in, nil
	}
}

func newPlacerFixture(checkout *fakeCheckout, unit string) *erpPurchasePlacer {
	return &erpPurchasePlacer{
		checkout: checkout,
		quote:    quoteAt(unit),
		variants: fakeVariants{rows: map[string]erp.VariantOfferRow{testVariantUUID: {ID: 11, OrganizationID: 20, PublicID: testVariantUUID, Active: true, InStock: true}}},
	}
}

func placeRequest(unit string) erp.PurchaseRequest {
	line := erp.PurchaseLine{VariantID: testVariantUUID, Quantity: 12}
	if unit != "" {
		price := money.MustParse(unit)
		line.UnitPrice = &price
	}
	return erp.PurchaseRequest{BuyerOrgID: 7, BuyingUserID: 5, BranchID: 41, BuyerOrgType: "pharmacy", Lines: []erp.PurchaseLine{line}, Notes: "[ERP:orgasoft:R1]"}
}

// TestErpPurchasePlacer resolves the variant UUID, quotes the line, and checks
// out the quoted input with the buyer's branch and organization.
func TestErpPurchasePlacer(t *testing.T) {
	checkout := &fakeCheckout{order: &commerce.Order{ID: 900, OrderNumber: "20871", TotalAmount: money.MustParse("656.28")}}
	got, err := newPlacerFixture(checkout, "54.69").Place(context.Background(), placeRequest("54.69"))
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if got.OrderID != 900 || got.OrderNumber != "20871" || got.Total.String() != "656.28" {
		t.Fatalf("placed = %+v", got)
	}
	if len(checkout.got) != 1 {
		t.Fatalf("checkout calls = %d, want 1", len(checkout.got))
	}
	in := checkout.got[0]
	if in.CustomerID != 5 || in.CustomerOrgID != 7 || in.CustomerOrgType != "pharmacy" || in.BranchID == nil || *in.BranchID != 41 || in.Notes != "[ERP:orgasoft:R1]" {
		t.Fatalf("checkout input = %+v", in)
	}
	if len(in.Items) != 1 || in.Items[0].ProductVariantID == nil || *in.Items[0].ProductVariantID != 11 || in.Items[0].VendorOrgID != 20 {
		t.Fatalf("items = %+v", in.Items)
	}
	if in.Items[0].UnitPrice.String() != "54.69" {
		t.Fatalf("quoted unit price = %s, want the quote's 54.69", in.Items[0].UnitPrice)
	}
}

func TestErpPurchasePlacerPriceMismatchCarriesTheQuote(t *testing.T) {
	checkout := &fakeCheckout{}
	_, err := newPlacerFixture(checkout, "54.69").Place(context.Background(), placeRequest("60.00"))
	var e *erp.Error
	if !errors.As(err, &e) || e.Code != "ERP_PO_PRICE_MISMATCH" {
		t.Fatalf("err = %v, want ERP_PO_PRICE_MISMATCH", err)
	}
	lines := e.Details["lines"].([]map[string]any)
	if lines[0]["sentPrice"] != "60.00" || lines[0]["correctPrice"] != "54.69" {
		t.Fatalf("lines = %+v", lines)
	}
	if len(checkout.got) != 0 {
		t.Fatal("no order may be submitted after a price mismatch")
	}
}

func TestErpPurchasePlacerRefusalBecomesPoRefused(t *testing.T) {
	checkout := &fakeCheckout{err: apperr.Conflict("quota.exhausted", "quota")}
	_, err := newPlacerFixture(checkout, "54.69").Place(context.Background(), placeRequest(""))
	var e *erp.Error
	if !errors.As(err, &e) || e.Code != "ERP_PO_REFUSED" || e.Details["reason"] != "quota.exhausted" {
		t.Fatalf("err = %v, want ERP_PO_REFUSED with reason quota.exhausted", err)
	}
}

func TestErpPurchasePlacerWalletBecomesPaymentNotSupported(t *testing.T) {
	checkout := &fakeCheckout{err: apperr.Conflict("checkout.wallet_on_website", "wallet")}
	_, err := newPlacerFixture(checkout, "54.69").Place(context.Background(), placeRequest(""))
	var e *erp.Error
	if !errors.As(err, &e) || e.Code != "ERP_PO_PAYMENT_NOT_SUPPORTED" {
		t.Fatalf("err = %v, want ERP_PO_PAYMENT_NOT_SUPPORTED", err)
	}
}
