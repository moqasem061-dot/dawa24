package pages_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestFilterUserPaymentMethodsByProvider(t *testing.T) {
	methods := []*billing.UserPaymentMethod{
		{ID: 1, Provider: "bank", AccountIdentifier: "CIB 1234"},
		{ID: 2, Provider: "instapay", AccountIdentifier: "user@instapay"},
		{ID: 3, Provider: "bank", AccountIdentifier: "NBE 5678"},
		{ID: 4, Provider: "wallet", AccountIdentifier: "01012345678"},
		nil,
	}

	banks := pages.FilterUserPaymentMethodsByProvider(methods, "bank")
	if len(banks) != 2 {
		t.Fatalf("expected 2 bank methods, got %d", len(banks))
	}
	if banks[0].ID != 1 || banks[1].ID != 3 {
		t.Errorf("unexpected bank methods: %v", banks)
	}

	instapay := pages.FilterUserPaymentMethodsByProvider(methods, "instapay")
	if len(instapay) != 1 || instapay[0].ID != 2 {
		t.Fatalf("expected instapay method 2, got %v", instapay)
	}

	wallets := pages.FilterUserPaymentMethodsByProvider(methods, "wallet")
	if len(wallets) != 1 || wallets[0].ID != 4 {
		t.Fatalf("expected wallet method 4, got %v", wallets)
	}

	cards := pages.FilterUserPaymentMethodsByProvider(methods, "card")
	if len(cards) != 0 {
		t.Fatalf("expected 0 card methods, got %d", len(cards))
	}
}

func TestHasDefaultPaymentMethod(t *testing.T) {
	noDefaults := []*billing.UserPaymentMethod{
		{ID: 1, IsDefault: false},
		{ID: 2, IsDefault: false},
		nil,
	}
	if pages.HasDefaultPaymentMethod(noDefaults) {
		t.Errorf("expected false, got true")
	}

	withDefault := []*billing.UserPaymentMethod{
		{ID: 1, IsDefault: false},
		{ID: 2, IsDefault: true},
	}
	if !pages.HasDefaultPaymentMethod(withDefault) {
		t.Errorf("expected true, got false")
	}
}
