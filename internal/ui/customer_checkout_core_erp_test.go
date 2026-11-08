package ui

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
)

// S5: the checkout plan stops while an ERP supplier has no customer code for the
// buyer, and never for a supplier without ERP.
func TestErpPlanCheckoutMissingCode(t *testing.T) {
	const erpVendor, plainVendor, buyerOrg, buyer int64 = 700, 800, 300, 41
	actor := customerActor(buyer, buyerOrg)
	items := []commerce.CheckoutLineItem{{VendorOrgID: erpVendor}, {VendorOrgID: plainVendor}}
	store := newCodeStore()
	store.erpSuppliers[erpVendor] = true
	h := codeHandler(store, true)

	f := h.erpMissingCodeFailure(context.Background(), actor, items)
	if f == nil || f.Back != "/checkout?erp_code=1" || f.Message != "" {
		t.Fatalf("failure = %+v, want the erp_code=1 return with no message", f)
	}

	store.addCodeLink(900, buyer, pointerOrNil(buyerOrg), erpVendor, "C-7", org.UserOrgStatusApproved)
	if f := h.erpMissingCodeFailure(context.Background(), actor, items); f != nil {
		t.Fatalf("with a code for the ERP supplier the plan continues, got %+v", f)
	}

	if f := codeHandler(newCodeStore(), false).erpMissingCodeFailure(context.Background(), actor, items); f != nil {
		t.Fatalf("without an ERP service the plan is unchanged, got %+v", f)
	}
}
