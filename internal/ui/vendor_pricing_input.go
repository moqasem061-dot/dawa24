package ui

import (
	"net/url"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func vendorInputDiscount(form url.Values, public money.Amount) (money.Amount, error) {
	mode := form.Get("discount_input_type")
	if mode == "sale_price" {
		sale, err := money.Parse(strings.TrimSpace(form.Get("sale_price")))
		if err != nil {
			return money.Zero, err
		}
		return money.DiscountFromSalePrice(public, sale)
	}
	if mode != "" && mode != "percentage" {
		return money.Zero, money.ErrInvalidFormat
	}
	raw := strings.TrimSpace(form.Get("discount"))
	if raw == "" {
		return money.Zero, nil
	}
	discount, err := money.Parse(raw)
	if err != nil || discount.IsNegative() || discount.Minor() > 10000 {
		return money.Zero, money.ErrInvalidFormat
	}
	return discount, nil
}
