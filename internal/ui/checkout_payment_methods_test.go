package ui_test

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
)

type mockBillingPaymentMethodsRepo struct {
	billing.Repository
	methods map[string]*billing.PlatformPaymentMethod
}

func (m *mockBillingPaymentMethodsRepo) ListPlatformPaymentMethods(_ context.Context, onlyActive bool) ([]*billing.PlatformPaymentMethod, error) {
	var result []*billing.PlatformPaymentMethod
	for _, pm := range m.methods {
		if !onlyActive || pm.IsActive {
			result = append(result, pm)
		}
	}
	return result, nil
}

func (m *mockBillingPaymentMethodsRepo) GetPlatformPaymentMethod(_ context.Context, id string) (*billing.PlatformPaymentMethod, error) {
	if pm, ok := m.methods[id]; ok {
		return pm, nil
	}
	return nil, nil
}

func (m *mockBillingPaymentMethodsRepo) TogglePlatformPaymentMethodCheckout(_ context.Context, id string, enabled bool) error {
	if pm, ok := m.methods[id]; ok {
		pm.IsCheckoutEnabled = enabled
	}
	return nil
}

func (m *mockBillingPaymentMethodsRepo) TogglePlatformPaymentMethod(_ context.Context, id string, active bool) error {
	if pm, ok := m.methods[id]; ok {
		pm.IsActive = active
	}
	return nil
}

func TestTogglePlatformPaymentMethodCheckout(t *testing.T) {
	repo := &mockBillingPaymentMethodsRepo{
		methods: map[string]*billing.PlatformPaymentMethod{
			"wallet": {ID: "wallet", IsActive: true, IsCheckoutEnabled: false},
			"cod":    {ID: "cod", IsActive: true, IsCheckoutEnabled: true},
		},
	}
	svc := billing.NewService(repo, nil)

	// Verify initial state: wallet is disabled for checkout
	pm, err := svc.GetPlatformPaymentMethod(context.Background(), "wallet")
	if err != nil || pm == nil {
		t.Fatalf("expected wallet method, got error: %v", err)
	}
	if pm.IsCheckoutEnabled {
		t.Errorf("expected wallet checkout to be disabled initially")
	}

	// Toggle wallet to enabled
	if err := svc.TogglePlatformPaymentMethodCheckout(context.Background(), "wallet", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.methods["wallet"].IsCheckoutEnabled {
		t.Errorf("expected wallet checkout to be enabled after toggle")
	}

	// Toggle back to disabled
	if err := svc.TogglePlatformPaymentMethodCheckout(context.Background(), "wallet", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.methods["wallet"].IsCheckoutEnabled {
		t.Errorf("expected wallet checkout to be disabled after second toggle")
	}
}
