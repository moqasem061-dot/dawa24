package ingest

import (
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// Turning a read row into the two prices the catalogue stores.
//
// A supplier states a public price and a discount; catalog.product_variants
// stores the list price and the discount as a percentage, and the pharmacy pays
// the difference. Getting that wrong in either direction is not a rounding
// error — it is the vendor's margin.

// listAndDiscount converts the row's reconciled prices into the pair the
// variant table stores: the list price, and the discount as a percentage.
//
// The percentage is derived from the two prices rather than from the stated
// one, so that what a pharmacy is charged always equals what the supplier's
// file said the net was — even where the stated percentage and the net
// disagreed by a piastre, which they routinely do.
func listAndDiscount(row *productmatch.Row) (list, discount money.Amount) {
	if row == nil {
		return money.Zero, money.Zero
	}
	list = row.PublicPrice
	if !list.IsPositive() {
		return row.NetPrice, money.Zero
	}
	if row.DiscountBps > 0 {
		return list, money.FromMinor(row.DiscountBps)
	}
	net := row.NetPrice
	if !net.IsPositive() || net.Minor() >= list.Minor() {
		return list, money.Zero
	}
	diff, err := list.Sub(net)
	if err != nil {
		return list, money.Zero
	}
	pctBps := diff.Minor() * 10000 / list.Minor()
	return list, money.FromMinor(pctBps)
}
