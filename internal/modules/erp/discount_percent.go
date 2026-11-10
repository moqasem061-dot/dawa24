package erp

import "github.com/moqasem061-dot/dawa24/internal/shared/money"

// DiscountPercent is the discount, as a percentage, that takes the list price
// down to the unit price. Orgasoft's feed shows it as "discount": 21.00 for a
// public price of 265.00 and a unit price of 209.35. Integer math on minor units
// only, rounded half up to two decimals of a percent. A list price that is not
// positive, or a unit price at or above it, gives 0.
func DiscountPercent(list, unit money.Amount) money.Amount {
	l, u := list.Minor(), unit.Minor()
	if l <= 0 || u >= l {
		return money.Zero
	}
	hundredths := ((l-u)*10000 + l/2) / l
	return money.FromMinor(hundredths)
}
