package ui

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func TestCoveringVendorsForFailsClosedForSelectedBranchWithoutCoverageService(t *testing.T) {
	h := &UIHandler{}

	vendors, evaluated := h.coveringVendorsFor(context.Background(), 73)
	if !evaluated {
		t.Fatal("selected branch must keep coverage filtering active when the service is unavailable")
	}
	if len(vendors) != 0 {
		t.Fatalf("vendors = %v, want empty fail-closed set", vendors)
	}
}

func TestCoveringVendorsForDoesNotApplyToBrowsingWithoutBranch(t *testing.T) {
	h := &UIHandler{}

	vendors, evaluated := h.coveringVendorsFor(context.Background(), 0)
	if evaluated || vendors != nil {
		t.Fatalf("branchless browsing = (%v, %v), want (nil, false)", vendors, evaluated)
	}
}

func TestResolveCheckoutBranchUsesShellSelectionOverForm(t *testing.T) {
	h := &UIHandler{}
	actor := authctx.Actor{OrganizationID: 188}
	ctx := authctx.WithBuyingBranch(context.Background(), authctx.BuyingBranch{
		Active: func() *int64 { id := int64(73); return &id }(),
	})

	got := h.resolveCheckoutBranch(ctx, actor, "69")
	if got == nil || *got != 73 {
		t.Fatalf("checkout branch = %v, want shell-selected branch 73", got)
	}
}

func TestResolveCheckoutBranchRejectsFormWhenShellHasNoBranch(t *testing.T) {
	h := &UIHandler{}
	actor := authctx.Actor{OrganizationID: 188}
	ctx := authctx.WithBuyingBranch(context.Background(), authctx.BuyingBranch{})

	if got := h.resolveCheckoutBranch(ctx, actor, "69"); got != nil {
		t.Fatalf("checkout accepted hand-posted branch %v without a shell selection", *got)
	}
}
