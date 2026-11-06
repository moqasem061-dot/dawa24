package main

import (
	"context"
	"errors"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

func checkoutEnabledMethods(_ context.Context, id string) (*billing.PlatformPaymentMethod, error) {
	switch id {
	case "cod", "bank":
		return &billing.PlatformPaymentMethod{ID: id, IsActive: true, IsCheckoutEnabled: true}, nil
	case "inactive":
		return &billing.PlatformPaymentMethod{ID: id, IsCheckoutEnabled: true}, nil
	case "deposit_only":
		return &billing.PlatformPaymentMethod{ID: id, IsActive: true}, nil
	default:
		return nil, errors.New("not found")
	}
}

func buyerAddresses(_ context.Context, userID int64) ([]*identity.UserAddress, error) {
	if userID == 7 {
		return []*identity.UserAddress{{ID: 70}, {ID: 71}}, nil
	}
	return []*identity.UserAddress{{ID: 80}}, nil
}

func addressID(id int64) *int64 { return &id }

func TestCheckBuyerChoices(t *testing.T) {
	cases := []struct {
		name       string
		method     string
		address    *int64
		wantCode   string
		wantMethod string
	}{
		{name: "no method defaults to cash on delivery like the website", wantMethod: "cod"},
		{name: "enabled method is kept", method: "bank", wantMethod: "bank"},
		{name: "surrounding spaces are dropped", method: " bank ", wantMethod: "bank"},
		{name: "unknown method is refused", method: "free_order", wantCode: "checkout.payment_method_unknown"},
		{name: "inactive method is refused", method: "inactive", wantCode: "checkout.payment_method_unavailable"},
		{name: "method not enabled for checkout is refused", method: "deposit_only", wantCode: "checkout.payment_method_unavailable"},
		{name: "buyer's own address is accepted", address: addressID(71), wantMethod: "cod"},
		{name: "another user's address is refused", address: addressID(80), wantCode: "checkout.address_invalid"},
		{name: "address that does not exist is refused", address: addressID(9999), wantCode: "checkout.address_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolver := apiQuoteResolver{paymentMethod: checkoutEnabledMethods, addresses: buyerAddresses}
			input := commerce.CheckoutInput{CustomerID: 7, PaymentMethod: tc.method, UserAddressID: tc.address}
			err := resolver.checkBuyerChoices(context.Background(), &input)
			if tc.wantCode != "" {
				var appErr *apperr.Error
				if !errors.As(err, &appErr) || appErr.Code != tc.wantCode {
					t.Fatalf("want %s, got %v", tc.wantCode, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected refusal: %v", err)
			}
			if input.PaymentMethod != tc.wantMethod {
				t.Fatalf("payment method: want %q, got %q", tc.wantMethod, input.PaymentMethod)
			}
		})
	}
}

func TestCheckBuyerChoicesDropsNonPositiveAddress(t *testing.T) {
	resolver := apiQuoteResolver{paymentMethod: checkoutEnabledMethods, addresses: buyerAddresses}
	input := commerce.CheckoutInput{CustomerID: 7, UserAddressID: addressID(0)}
	if err := resolver.checkBuyerChoices(context.Background(), &input); err != nil {
		t.Fatal(err)
	}
	if input.UserAddressID != nil {
		t.Fatalf("expected no address, got %d", *input.UserAddressID)
	}
}

func TestCheckBuyerChoicesFailsClosedWhenNotWired(t *testing.T) {
	resolver := apiQuoteResolver{}
	if err := resolver.checkBuyerChoices(context.Background(), &commerce.CheckoutInput{CustomerID: 7}); err == nil {
		t.Fatal("expected a refusal when the checks are not wired")
	}
}

func TestCheckBuyerChoicesReturnsAddressLookupFailure(t *testing.T) {
	failure := errors.New("database unavailable")
	resolver := apiQuoteResolver{
		paymentMethod: checkoutEnabledMethods,
		addresses:     func(context.Context, int64) ([]*identity.UserAddress, error) { return nil, failure },
	}
	input := commerce.CheckoutInput{CustomerID: 7, UserAddressID: addressID(70)}
	if err := resolver.checkBuyerChoices(context.Background(), &input); !errors.Is(err, failure) {
		t.Fatalf("expected the lookup failure, got %v", err)
	}
}
