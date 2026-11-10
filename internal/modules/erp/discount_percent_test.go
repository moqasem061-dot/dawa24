package erp

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func TestDiscountPercent(t *testing.T) {
	cases := []struct {
		name       string
		list, unit string
		want       string
	}{
		{"orgasoft spiercoda", "265.00", "209.35", "21.00"},
		{"orgasoft 385", "385.00", "300.30", "22.00"},
		{"orgasoft 220", "220.00", "154.00", "30.00"},
		{"orgasoft rounding", "62.50", "54.69", "12.50"},
		{"no list price", "0", "10", "0.00"},
		{"unit equals list", "10", "10", "0.00"},
		{"unit above list", "10", "12", "0.00"},
		{"no discount", "100.00", "100.00", "0.00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DiscountPercent(money.MustParse(tc.list), money.MustParse(tc.unit))
			if got.String() != tc.want {
				t.Fatalf("DiscountPercent(%s, %s) = %s, want %s", tc.list, tc.unit, got.String(), tc.want)
			}
		})
	}
}
