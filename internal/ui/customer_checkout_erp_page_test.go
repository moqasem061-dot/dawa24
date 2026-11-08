package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// S5: the checkout page decides the code modal from the cart and the buyer's
// links. A query string such as erp_code=1 never opens it, and with nothing
// missing no modal renders.
func TestErpCheckoutPageModal(t *testing.T) {
	const erpVendor, buyerOrg, buyer int64 = 700, 300, 41
	actor := customerActor(buyer, buyerOrg)
	store := newCodeStore()
	store.erpSuppliers[erpVendor] = true
	h := codeHandler(store, true)
	cart := &commerce.Cart{Items: []*commerce.CartItem{{OrganizationID: erpVendor}}}

	needs, err := h.erpMissingCodeNeeds(context.Background(), actor, cart, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(needs) != 1 || needs[0].VendorOrgID != erpVendor {
		t.Fatalf("needs = %+v, want the ERP supplier %d", needs, erpVendor)
	}

	render := func(extras pages.CheckoutExtras) string {
		var buf bytes.Buffer
		if err := pages.CustomerCheckoutWithExtras(&commerce.Cart{}, nil, nil, nil, nil, "en", "ltr", extras).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	with := render(pages.CheckoutExtras{MissingERPCodes: needs})
	if !strings.Contains(with, "erp-code-modal") || !strings.Contains(with, "/checkout/erp-customer-code") {
		t.Fatalf("page with a missing code must render the modal and post to /checkout/erp-customer-code")
	}
	without := render(pages.CheckoutExtras{})
	if strings.Contains(without, "erp-code-modal") {
		t.Fatalf("page with nothing missing must not render the modal")
	}

	store.addCodeLink(900, buyer, pointerOrNil(buyerOrg), erpVendor, "C-7", org.UserOrgStatusApproved)
	after, err := h.erpMissingCodeNeeds(context.Background(), actor, cart, "en")
	if err != nil || len(after) != 0 {
		t.Fatalf("after the code is saved needs = %+v err=%v, want none", after, err)
	}
}
