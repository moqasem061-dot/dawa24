// Pricing for offer lines — the single EffectivePrice resolver.
//
// Rebuild V2 §3.1 — every price shown for an offer product must come through
// this function. Precedence, from strongest to weakest:
//
//  1. custom_price                — the vendor overrides the list price entirely
//  2. custom_discount_amount      — fixed discount off the list price (EGP)
//  3. custom_discount_percentage  — percent discount off the list price
//  4. the offer-level discount (DiscountType/DiscountValue); its percentage
//     semantics mirror the rest of the system: DiscountValue 15.00 → 1500 bps
//
// A line never ends up negative: results are floored at zero.
package promo

import (
	"math"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// DiscountSource says which rule produced the final price.
type DiscountSource string

const (
	SourceCustomPrice   DiscountSource = "custom_price"
	SourceCustomAmount  DiscountSource = "custom_amount"
	SourceCustomPercent DiscountSource = "custom_percent"
	SourceOffer         DiscountSource = "offer"
	SourceNone          DiscountSource = "none"
)

// DiscountBreakdown is the full derivation of one line's final price, for
// display (i18n.TDefault("w4_mod.15_244")) and for order-line persistence later.
type DiscountBreakdown struct {
	ListPrice      money.Amount
	FinalPrice     money.Amount
	DiscountAmount money.Amount
	DiscountBPS    int64 // effective basis points; 0 for fixed/none
	Source         DiscountSource
}

// EffectivePrice resolves the price a customer actually pays for a variant
// inside an offer. listPrice is the variant's list price from catalog;
// op and o may be nil-safe: a nil product line or offer yields the list price.
func EffectivePrice(listPrice money.Amount, op *OfferProduct, o *Offer) (money.Amount, DiscountBreakdown) {
	bd := DiscountBreakdown{ListPrice: listPrice, FinalPrice: listPrice, Source: SourceNone}

	if op != nil && op.CustomPrice != nil && op.CustomPrice.IsPositive() {
		bd.FinalPrice = *op.CustomPrice
		bd.DiscountAmount = money.Zero
		bd.Source = SourceCustomPrice
		return bd.FinalPrice, bd
	}
	if op != nil && op.CustomDiscountAmount != nil && op.CustomDiscountAmount.IsPositive() {
		return applyAmount(bd, *op.CustomDiscountAmount)
	}
	if op != nil && op.CustomDiscountPercent != nil && *op.CustomDiscountPercent > 0 {
		return applyBPS(bd, percentToBPS(*op.CustomDiscountPercent))
	}

	if o != nil && o.DiscountValue.IsPositive() {
		switch o.DiscountType {
		case DiscountFixed:
			final, bdRes := applyAmount(bd, o.DiscountValue)
			bdRes.Source = SourceOffer
			return final, bdRes
		case DiscountPercentage:
			final, bdRes := applyBPS(bd, o.DiscountValue.Minor())
			bdRes.Source = SourceOffer
			return final, bdRes
		}
	}

	return bd.FinalPrice, bd
}

// applyAmount subtracts an exact discount, flooring the result at zero.
func applyAmount(bd DiscountBreakdown, amount money.Amount) (money.Amount, DiscountBreakdown) {
	final, err := bd.ListPrice.Sub(amount)
	if err != nil || final.IsNegative() {
		final = money.Zero
	}
	bd.FinalPrice = final
	discount, _ := bd.ListPrice.Sub(final)
	bd.DiscountAmount = discount
	bd.Source = SourceCustomAmount
	return final, bd
}

// applyBPS subtracts a percentage discount by basis points.
func applyBPS(bd DiscountBreakdown, bps int64) (money.Amount, DiscountBreakdown) {
	discount := bd.ListPrice.ApplyPercent(bps)
	if discount.IsNegative() {
		discount = money.Zero
	}
	bd.DiscountAmount = discount
	bd.DiscountBPS = bps
	bd.Source = SourceCustomPercent
	final, err := bd.ListPrice.Sub(discount)
	if err != nil || final.IsNegative() {
		final = money.Zero
	}
	bd.FinalPrice = final
	return final, bd
}

// percentToBPS turns the stored two-decimal percent (15.00 = 15%) into basis
// points (1500), rounding half away from zero like the rest of the codebase.
func percentToBPS(pct float64) int64 {
	return int64(math.Round(pct * 100))
}

// HasCustomPrice reports whether the line fully overrides the list price.
func (op *OfferProduct) HasCustomPrice() bool {
	return op != nil && op.CustomPrice != nil
}
