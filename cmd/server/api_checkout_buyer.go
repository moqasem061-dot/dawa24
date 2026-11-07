package main

import (
	"context"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// checkBuyerChoices holds the REST checkout to the rules the website applies to
// the buyer's own selections: the payment method must be one the platform enables
// for checkout, and a delivery address must belong to the buyer placing the order.
func (q *apiQuoteResolver) checkBuyerChoices(ctx context.Context, requested *commerce.CheckoutInput) error {
	if q.paymentMethod == nil || q.addresses == nil {
		return quoteUnavailable()
	}
	method := strings.TrimSpace(requested.PaymentMethod)
	if method == "" {
		method = commerce.DefaultPaymentMethod
	}
	selected, err := q.paymentMethod(ctx, method)
	if err != nil || selected == nil {
		return apperr.Validation("checkout.payment_method_unknown", "طريقة الدفع المحددة غير معروفة. The selected payment method is not recognised.", nil)
	}
	if !selected.IsActive || !selected.IsCheckoutEnabled {
		return apperr.Validation("checkout.payment_method_unavailable", "طريقة الدفع المحددة غير متاحة حالياً. The selected payment method is not available.", nil)
	}
	requested.PaymentMethod = method

	if requested.UserAddressID == nil || *requested.UserAddressID <= 0 {
		// The order repository stores no address for a non-positive id.
		requested.UserAddressID = nil
		return nil
	}
	owned, err := q.addresses(database.AsSystem(ctx), requested.CustomerID)
	if err != nil {
		return err
	}
	if !ownsAddress(owned, *requested.UserAddressID) {
		return apperr.Forbidden("checkout.address_invalid", "عنوان التوصيل غير متاح. The delivery address is not available.")
	}
	return nil
}

func ownsAddress(owned []*identity.UserAddress, id int64) bool {
	for _, address := range owned {
		if address != nil && address.ID == id {
			return true
		}
	}
	return false
}

// apiBuyerChoices supplies checkBuyerChoices with the services that decide it.
// Billing is built after commerce, so the routes wire these in afterwards.
func (q *apiQuoteResolver) apiBuyerChoices(bill *billing.Service, id *identity.Service) {
	q.paymentMethod = bill.GetPlatformPaymentMethod
	q.addresses = id.ListAddresses
}
