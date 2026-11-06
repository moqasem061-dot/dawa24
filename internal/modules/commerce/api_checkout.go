package commerce

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// APIQuoteResolver resolves all commercially significant values from trusted sources.
type APIQuoteResolver func(context.Context, CheckoutInput) (CheckoutInput, error)

func (s *Service) SetAPIQuoteResolver(resolve APIQuoteResolver) { s.apiQuote = resolve }

// CheckoutAPI places an order from the REST checkout. Every amount that affects
// money is re-derived on the server; the caller's figures are only compared
// against that result, and any difference fails the whole request before an
// order is written.
func (s *Service) CheckoutAPI(ctx context.Context, submitted CheckoutInput) (*Order, error) {
	if submitted.IsNegotiation {
		return nil, apperr.Conflict("checkout.negotiation_on_website", "يرجى تقديم طلب التفاوض من الموقع. Please submit price negotiations on the website.")
	}
	// The website debits the wallet after a funds check it performs at checkout.
	// The REST path has no such check, so it cannot place a wallet order safely.
	if IsWalletPayment(submitted.PaymentMethod) {
		return nil, apperr.Conflict("checkout.wallet_on_website", "الدفع من المحفظة متاح من الموقع فقط. Wallet payment is available on the website only.")
	}
	for _, line := range submitted.Items {
		if line.ProductVariantID == nil || *line.ProductVariantID <= 0 {
			return nil, apperr.Conflict("checkout.unverifiable_on_website", "يرجى طلب هذا المنتج أو العرض من الموقع. Please order this product or offer on the website.")
		}
	}
	if s.apiQuote == nil {
		return nil, apperr.Unavailable("checkout.quote", errors.New("API quote resolver is not configured"))
	}
	if s.availability == nil {
		return nil, apperr.Unavailable("checkout.availability", errors.New("availability probe is not configured"))
	}
	request := submitted
	request.Items = append([]CheckoutLineItem(nil), submitted.Items...)
	request.VendorShippingFees = maps.Clone(submitted.VendorShippingFees)
	request.VendorBranchIDs = maps.Clone(submitted.VendorBranchIDs)
	quote, err := s.apiQuote(ctx, request)
	if err != nil {
		return nil, err
	}
	if len(quote.Items) != len(submitted.Items) {
		return nil, apperr.Internal(errors.New("quote line count mismatch"))
	}
	for i, line := range quote.Items {
		if line.Quantity != submitted.Items[i].Quantity {
			return nil, apperr.Internal(errors.New("quote quantity mismatch"))
		}
		client := submitted.Items[i]
		if line.UnitPrice != client.UnitPrice || ((client.SubmittedDiscount || !client.DiscountAmount.IsZero()) && line.DiscountAmount != client.DiscountAmount) ||
			((client.SubmittedListPrice || !client.ListPrice.IsZero()) && line.ListPrice != client.ListPrice) ||
			((client.SubmittedOriginalPrice || !client.OriginalPrice.IsZero()) && line.OriginalPrice != client.OriginalPrice) ||
			((client.SubmittedOriginalDiscount || !client.OriginalDiscount.IsZero()) && line.OriginalDiscount != client.OriginalDiscount) ||
			(client.CostPrice != nil && (line.CostPrice == nil || *line.CostPrice != *client.CostPrice)) || !client.ProposedUnitPrice.IsZero() {
			return nil, apiPriceChanged(quote)
		}
	}
	hasShippingTotal := submitted.SubmittedShippingFee || !submitted.ShippingFee.IsZero()
	if (hasShippingTotal && quote.ShippingFee != submitted.ShippingFee) || quote.TaxAmount != submitted.TaxAmount {
		return nil, apiPriceChanged(quote)
	}
	if !hasShippingTotal && submitted.VendorShippingFees == nil && !quote.ShippingFee.IsZero() {
		return nil, apiPriceChanged(quote)
	}
	if submitted.VendorShippingFees != nil && !matchingShippingFees(quote.VendorShippingFees, submitted.VendorShippingFees) {
		return nil, apiPriceChanged(quote)
	}
	quote.CustomerID, quote.CustomerOrgID, quote.CustomerOrgType = submitted.CustomerID, submitted.CustomerOrgID, submitted.CustomerOrgType
	quote.PaymentStatus = PaymentUnpaid
	return s.checkout(ctx, quote, func(context.Context, *CheckoutInput) error { return nil })
}

func matchingShippingFees(quoted, submitted map[int64]money.Amount) bool {
	for vendorID, fee := range submitted {
		if expected, ok := quoted[vendorID]; !ok || expected != fee {
			return false
		}
	}
	for vendorID, fee := range quoted {
		if !fee.IsZero() && submitted[vendorID] != fee {
			return false
		}
	}
	return true
}

func apiPriceChanged(quote CheckoutInput) error {
	err := apperr.Conflict("checkout.price_changed", "تغير سعر الطلب. يرجى مراجعة السعر وتأكيده قبل المحاولة مجدداً. The order price changed. Please review the current amount and confirm before retrying.")
	err.Fields = map[string]string{"shipping_fee": quote.ShippingFee.String(), "tax_amount": quote.TaxAmount.String()}
	for vendorID, fee := range quote.VendorShippingFees {
		err.Fields[fmt.Sprintf("vendor_shipping_fees.%d", vendorID)] = fee.String()
	}
	for i, line := range quote.Items {
		err.Fields[fmt.Sprintf("items.%d.unit_price", i)] = line.UnitPrice.String()
		err.Fields[fmt.Sprintf("items.%d.discount_amount", i)] = line.DiscountAmount.String()
	}
	return err
}
