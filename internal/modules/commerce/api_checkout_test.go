package commerce

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func TestAPIQuoteComparesSubmittedPricesBeforeOrderCreation(t *testing.T) {
	for _, tc := range []struct {
		name, submitted string
		wantOrder       bool
	}{
		{"unchanged", "90.00", true}, {"underpriced", "1.00", false}, {"stale_price", "95.00", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockCommerceRepo()
			service := cartService(repo)
			variantID := int64(5)
			branchID := int64(3)
			probe := healthyProbe()
			probe.variant.ID = variantID
			probe.variant.OrganizationID = 10
			probe.vendor.ID = 10
			service.SetAvailabilityProbe(probe)
			service.SetAPIQuoteResolver(func(_ context.Context, request CheckoutInput) (CheckoutInput, error) {
				request.Items[0].UnitPrice = money.MustParse("90.00")
				request.Items[0].ListPrice = money.MustParse("100.00")
				request.Items[0].DiscountAmount = money.MustParse("20.00")
				request.Items[0].VendorOrgID = 10
				return request, nil
			})
			order, err := service.CheckoutAPI(context.Background(), CheckoutInput{CustomerID: 1, CustomerOrgID: 99, BranchID: &branchID, PaymentStatus: PaymentPaid, Items: []CheckoutLineItem{{ProductVariantID: &variantID, Quantity: 2, UnitPrice: money.MustParse(tc.submitted)}}})
			if (err == nil) != tc.wantOrder {
				t.Fatalf("order=%v err=%v", order, err)
			}
			if !tc.wantOrder && len(repo.orders) != 0 {
				t.Fatal("order created after price mismatch")
			}
			if !tc.wantOrder {
				priceErr, ok := apperr.As(err)
				if !ok || priceErr.Code != "checkout.price_changed" || priceErr.Fields["items.0.unit_price"] != "90.00" || priceErr.Fields["items.0.discount_amount"] != "20.00" {
					t.Fatalf("missing authoritative review prices: %v", err)
				}
			}
			if tc.wantOrder && (order.TotalAmount != money.MustParse("180.00") || order.PaymentStatus != PaymentUnpaid || order.Shipments[0].OrganizationID != 10) {
				t.Fatalf("incorrect order: %+v", order)
			}
		})
	}
}

func TestAPICheckoutRejectsNegotiationWithoutCreatingOrder(t *testing.T) {
	repo := newMockCommerceRepo()
	service := cartService(repo)
	called := false
	service.SetAPIQuoteResolver(func(context.Context, CheckoutInput) (CheckoutInput, error) {
		called = true
		return CheckoutInput{}, nil
	})
	_, err := service.CheckoutAPI(context.Background(), CheckoutInput{CustomerID: 1, IsNegotiation: true})
	if err == nil || called || len(repo.orders) != 0 {
		t.Fatalf("err=%v resolver called=%v orders=%d", err, called, len(repo.orders))
	}
}

func TestAPICheckoutRechecksStockAndQuota(t *testing.T) {
	for _, tc := range []struct {
		name               string
		stock, quota, used int
	}{
		{"stock", 1, 0, 0}, {"quota", 10, 2, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newQuotaRepo(map[int64]int{3: tc.used})
			service := cartService(repo)
			probe := healthyProbe()
			probe.variant.StockQty, probe.variant.QuotaLimit = tc.stock, tc.quota
			service.SetAvailabilityProbe(probe)
			service.SetAPIQuoteResolver(func(_ context.Context, request CheckoutInput) (CheckoutInput, error) { return request, nil })
			variantID, branchID := int64(10), int64(3)
			_, err := service.CheckoutAPI(context.Background(), CheckoutInput{CustomerID: 1, CustomerOrgID: 99, BranchID: &branchID, Items: []CheckoutLineItem{{VendorOrgID: 7, ProductVariantID: &variantID, Quantity: 2, UnitPrice: money.MustParse("10.00")}}})
			if err == nil || len(repo.orders) != 0 {
				t.Fatalf("err=%v orders=%d", err, len(repo.orders))
			}
		})
	}
}

func TestAPICheckoutRejectsIncorrectExplicitDiscount(t *testing.T) {
	repo := newMockCommerceRepo()
	service := cartService(repo)
	service.SetAvailabilityProbe(healthyProbe())
	service.SetAPIQuoteResolver(func(_ context.Context, request CheckoutInput) (CheckoutInput, error) {
		request.Items[0].DiscountAmount = money.MustParse("20.00")
		return request, nil
	})
	variantID := int64(10)
	_, err := service.CheckoutAPI(context.Background(), CheckoutInput{CustomerID: 1, Items: []CheckoutLineItem{{ProductVariantID: &variantID, Quantity: 2, UnitPrice: money.MustParse("90.00"), SubmittedDiscount: true}}})
	if err == nil || len(repo.orders) != 0 {
		t.Fatalf("err=%v orders=%d", err, len(repo.orders))
	}
}

func TestAPICheckoutRequiresTheBuyerToSupplyTheDeliveryAmount(t *testing.T) {
	for _, tc := range []struct {
		name      string
		fee       money.Amount
		explicit  bool
		perVendor map[int64]money.Amount
		allow     bool
	}{
		{"missing", money.Zero, false, nil, false},
		{"explicit_zero", money.Zero, true, nil, false},
		{"correct_total", money.MustParse("12.00"), true, nil, true},
		{"correct_per_vendor", money.Zero, false, map[int64]money.Amount{7: money.MustParse("12.00")}, true},
		{"wrong_per_vendor", money.Zero, false, map[int64]money.Amount{7: money.MustParse("1.00")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockCommerceRepo()
			service := cartService(repo)
			service.SetAvailabilityProbe(healthyProbe())
			service.SetAPIQuoteResolver(func(_ context.Context, request CheckoutInput) (CheckoutInput, error) {
				request.ShippingFee = money.MustParse("12.00")
				request.VendorShippingFees = map[int64]money.Amount{7: money.MustParse("12.00")}
				return request, nil
			})
			variantID, branchID := int64(10), int64(3)
			order, err := service.CheckoutAPI(context.Background(), CheckoutInput{CustomerID: 1, CustomerOrgID: 99, BranchID: &branchID, ShippingFee: tc.fee, SubmittedShippingFee: tc.explicit, VendorShippingFees: tc.perVendor, Items: []CheckoutLineItem{{ProductVariantID: &variantID, VendorOrgID: 7, Quantity: 1, UnitPrice: money.MustParse("10.00")}}})
			if (err == nil) != tc.allow {
				t.Fatalf("order=%+v err=%v", order, err)
			}
			if tc.allow && order.TotalAmount != money.MustParse("22.00") {
				t.Fatalf("total=%s", order.TotalAmount)
			}
			if !tc.allow && len(repo.orders) != 0 {
				t.Fatal("order created without agreement on delivery")
			}
		})
	}
}

func TestMatchingShippingFeesAllowsOmittedZeroCharges(t *testing.T) {
	quoted := map[int64]money.Amount{1: money.Zero, 2: money.MustParse("12.00")}
	if !matchingShippingFees(quoted, map[int64]money.Amount{2: money.MustParse("12.00")}) {
		t.Fatal("omitted zero charge should match")
	}
	if matchingShippingFees(quoted, map[int64]money.Amount{1: money.Zero}) {
		t.Fatal("omitted positive charge must not match")
	}
	if matchingShippingFees(quoted, map[int64]money.Amount{2: money.MustParse("1.00")}) {
		t.Fatal("incorrect charge must not match")
	}
}
