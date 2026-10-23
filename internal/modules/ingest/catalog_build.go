package ingest

import (
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
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
// A stated or normalized percentage is preserved. Older staged rows carrying
// only public and net prices derive their percentage with the same rounding.
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
	discount, err := money.DiscountFromSalePrice(list, net)
	if err != nil {
		return list, money.Zero
	}
	return list, discount
}
