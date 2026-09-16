package ui

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestCheckoutBranchItem_DisabledNeverChecked(t *testing.T) {
	ctx := context.Background()

	b1 := &pages.CheckoutBranchItem{
		Branch:      &org.Branch{ID: 1, Name: i18n.New("فرع القاهرة", "Cairo"), IsMain: true},
		IsAvailable: false,
		Reason:      "المورد لا يغطي هذا الفرع اليوم",
	}
	b2 := &pages.CheckoutBranchItem{
		Branch:      &org.Branch{ID: 2, Name: i18n.New("فرع الجيزة", "Giza")},
		IsAvailable: true,
	}
	branches := []*pages.CheckoutBranchItem{b1, b2}

	// b1 is IsMain, but IsAvailable is false -> must NEVER be checked
	if pages.IsCheckoutBranchChecked(ctx, b1, branches) {
		t.Errorf("expected disabled branch b1 to NOT be checked")
	}

	// b2 is available -> should be checked as the first available branch
	if !pages.IsCheckoutBranchChecked(ctx, b2, branches) {
		t.Errorf("expected available branch b2 to be checked")
	}
}

func TestCheckoutBranchItem_ActiveBranchUnavailableFallback(t *testing.T) {
	activeID := int64(2)
	// Active branch in buying context is #2 (e.g. Riyadh branch)
	ctx := authctx.WithBuyingBranch(context.Background(), authctx.BuyingBranch{
		Active:   &activeID,
		IsLocked: false,
	})

	b1 := &pages.CheckoutBranchItem{
		Branch:      &org.Branch{ID: 1, Name: i18n.New("فرع القاهرة", "Cairo"), IsMain: true},
		IsAvailable: true,
	}
	b2 := &pages.CheckoutBranchItem{
		Branch:      &org.Branch{ID: 2, Name: i18n.New("فرع الرياض", "Riyadh")},
		IsAvailable: false,
		Reason:      "المورد لا يغطي موقع فرعك في هذا اليوم",
	}
	branches := []*pages.CheckoutBranchItem{b1, b2}

	// Even though #2 is active in context, it's unavailable -> must NOT be checked
	if pages.IsCheckoutBranchChecked(ctx, b2, branches) {
		t.Errorf("expected active but unavailable branch b2 to NOT be checked")
	}

	// Branch #1 is available and main -> should be checked automatically
	if !pages.IsCheckoutBranchChecked(ctx, b1, branches) {
		t.Errorf("expected available fallback branch b1 to be checked")
	}
}

func TestCheckoutBranchItem_ActiveBranchAvailableSelected(t *testing.T) {
	activeID := int64(2)
	ctx := authctx.WithBuyingBranch(context.Background(), authctx.BuyingBranch{
		Active:   &activeID,
		IsLocked: false,
	})

	b1 := &pages.CheckoutBranchItem{
		Branch:      &org.Branch{ID: 1, Name: i18n.New("فرع القاهرة", "Cairo"), IsMain: true},
		IsAvailable: true,
	}
	b2 := &pages.CheckoutBranchItem{
		Branch:      &org.Branch{ID: 2, Name: i18n.New("فرع الجيزة", "Giza")},
		IsAvailable: true,
	}
	branches := []*pages.CheckoutBranchItem{b1, b2}

	// Branch #2 is active and available -> should be checked
	if !pages.IsCheckoutBranchChecked(ctx, b2, branches) {
		t.Errorf("expected active available branch b2 to be checked")
	}
	// Branch #1 is main, but not the active choice -> should NOT be checked
	if pages.IsCheckoutBranchChecked(ctx, b1, branches) {
		t.Errorf("expected b1 to NOT be checked when b2 is active and available")
	}
}

func TestHasAvailableCheckoutBranch(t *testing.T) {
	b1 := &pages.CheckoutBranchItem{
		Branch:      &org.Branch{ID: 1},
		IsAvailable: false,
	}
	b2 := &pages.CheckoutBranchItem{
		Branch:      &org.Branch{ID: 2},
		IsAvailable: false,
	}

	if pages.HasAvailableCheckoutBranch([]*pages.CheckoutBranchItem{b1, b2}) {
		t.Errorf("expected false when all branches unavailable")
	}

	b2.IsAvailable = true
	if !pages.HasAvailableCheckoutBranch([]*pages.CheckoutBranchItem{b1, b2}) {
		t.Errorf("expected true when at least one branch is available")
	}

	if pages.HasAvailableCheckoutBranch(nil) {
		t.Errorf("expected false for nil branches")
	}
}

