package compare

import (
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
)

// ParseImportPrice uses the shared decimal parser; monetary values never pass through floats.
func ParseImportPrice(raw string) money.Amount {
	decimal, err := sheet.Coerce(raw)
	if err != nil {
		return money.Zero
	}
	amount, err := money.Parse(decimal.Canonical)
	if err != nil || amount.IsNegative() || amount.Minor() >= maxPriceMinor {
		return money.Zero
	}
	return amount
}
