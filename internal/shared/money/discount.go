package money

import "errors"

var ErrInvalidSalePrice = errors.New("money: sale price must be between zero and a positive public price")

// DiscountFromSalePrice returns percentage points to two decimal places,
// rounded half away from zero. MulRatio avoids overflow for large prices.
func DiscountFromSalePrice(public, sale Amount) (Amount, error) {
	if !public.IsPositive() || sale.IsNegative() || sale.Minor() > public.Minor() {
		return Zero, ErrInvalidSalePrice
	}
	difference, err := public.Sub(sale)
	if err != nil {
		return Zero, err
	}
	return difference.MulRatio(10000, public.Minor())
}
