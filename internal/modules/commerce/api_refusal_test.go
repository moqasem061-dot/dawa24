package commerce

import (
	"context"
	"testing"
	"unicode"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// requireRefusal checks the stable code, that the message is bilingual (an
// Arabic part and a Latin part), and that the caller got an error at all.
func requireRefusal(t *testing.T, err error, code string) {
	t.Helper()
	apiErr, ok := apperr.As(err)
	if !ok || apiErr.Code != code {
		t.Fatalf("err=%v, want refusal code %q", err, code)
	}
	var arabic, latin bool
	for _, r := range apiErr.Msg {
		if unicode.Is(unicode.Arabic, r) {
			arabic = true
		}
		if r < 128 && unicode.IsLetter(r) {
			latin = true
		}
	}
	if !arabic || !latin {
		t.Fatalf("refusal message is not bilingual: %q", apiErr.Msg)
	}
}

// Wallet orders move money on confirmation and cancellation, and only the
// website does that. The REST checkout has no funds check, so it refuses them.
func TestAPICheckoutRefusesWalletPaymentWithoutCreatingOrder(t *testing.T) {
	for _, method := range []string{"wallet", "Wallet", " WALLET "} {
		t.Run("method_"+method, func(t *testing.T) {
			repo := newMockCommerceRepo()
			service := cartService(repo)
			service.SetAvailabilityProbe(healthyProbe())
			called := false
			service.SetAPIQuoteResolver(func(context.Context, CheckoutInput) (CheckoutInput, error) {
				called = true
				return CheckoutInput{}, nil
			})
			variantID, branchID := int64(10), int64(3)
			_, err := service.CheckoutAPI(context.Background(), CheckoutInput{
				CustomerID: 1, CustomerOrgID: 99, BranchID: &branchID, PaymentMethod: method,
				Items: []CheckoutLineItem{{ProductVariantID: &variantID, VendorOrgID: 7, Quantity: 1, UnitPrice: money.MustParse("10.00")}},
			})
			requireRefusal(t, err, "checkout.wallet_on_website")
			if called || len(repo.orders) != 0 {
				t.Fatalf("resolver called=%v orders=%d", called, len(repo.orders))
			}
		})
	}
}

// A standalone bundle has no variant, so no trusted price exists for it. The
// whole REST request is refused, including the lines that could be verified.
func TestAPICheckoutRefusesStandaloneBundleWithoutCreatingOrder(t *testing.T) {
	repo := newMockCommerceRepo()
	service := cartService(repo)
	service.SetAvailabilityProbe(healthyProbe())
	called := false
	service.SetAPIQuoteResolver(func(context.Context, CheckoutInput) (CheckoutInput, error) {
		called = true
		return CheckoutInput{}, nil
	})
	variantID, offerProductID, branchID := int64(10), int64(55), int64(3)
	_, err := service.CheckoutAPI(context.Background(), CheckoutInput{
		CustomerID: 1, CustomerOrgID: 99, BranchID: &branchID, PaymentMethod: "cod",
		Items: []CheckoutLineItem{
			{ProductVariantID: &variantID, VendorOrgID: 7, Quantity: 1, UnitPrice: money.MustParse("10.00")},
			{VendorOrgID: 7, OfferProductID: &offerProductID, Quantity: 1, UnitPrice: money.MustParse("30.00")},
		},
	})
	requireRefusal(t, err, "checkout.unverifiable_on_website")
	if called || len(repo.orders) != 0 {
		t.Fatalf("resolver called=%v orders=%d", called, len(repo.orders))
	}
}

// The website keeps selling standalone bundles, priced from the cart. Only the
// REST checkout refuses them, so this proves the website path is unchanged.
func TestWebsiteStandaloneBundleStillChecksOut(t *testing.T) {
	repo := newMockCommerceRepo()
	service := cartService(repo)
	service.SetAvailabilityProbe(healthyProbe())
	offerProductID, branchID := int64(55), int64(3)
	order, err := service.Checkout(context.Background(), CheckoutInput{
		CustomerID: 1, CustomerOrgID: 99, BranchID: &branchID, PaymentMethod: "cod",
		Items: []CheckoutLineItem{{VendorOrgID: 7, OfferProductID: &offerProductID, Quantity: 2, UnitPrice: money.MustParse("30.00"), DiscountAmount: money.MustParse("10.00")}},
	})
	if err != nil {
		t.Fatalf("website bundle checkout refused: %v", err)
	}
	if len(repo.orders) != 1 || order.TotalAmount != money.MustParse("50.00") {
		t.Fatalf("orders=%d total=%s", len(repo.orders), order.TotalAmount)
	}
}

// The website's wallet checkout is also unchanged: the same service call still
// creates the order with the authorized status the website sets.
func TestWebsiteWalletCheckoutStillCreatesAuthorizedOrder(t *testing.T) {
	repo := newMockCommerceRepo()
	service := cartService(repo)
	service.SetAvailabilityProbe(healthyProbe())
	variantID, branchID := int64(10), int64(3)
	order, err := service.Checkout(context.Background(), CheckoutInput{
		CustomerID: 1, CustomerOrgID: 99, BranchID: &branchID, PaymentMethod: "wallet", PaymentStatus: PaymentAuthorized,
		Items: []CheckoutLineItem{{ProductVariantID: &variantID, VendorOrgID: 7, Quantity: 1, UnitPrice: money.MustParse("10.00")}},
	})
	if err != nil {
		t.Fatalf("website wallet checkout refused: %v", err)
	}
	if len(repo.orders) != 1 || order.PaymentStatus != PaymentAuthorized {
		t.Fatalf("orders=%d payment=%s", len(repo.orders), order.PaymentStatus)
	}
}
