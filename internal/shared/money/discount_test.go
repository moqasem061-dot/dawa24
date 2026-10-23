package money

import "testing"

func TestDiscountFromSalePrice(t *testing.T) {
	for _, tc := range []struct {
		name, public, sale, want string
		invalid                  bool
	}{
		{"standard", "100", "80", "20.00", false},
		{"no discount", "125.50", "125.50", "0.00", false},
		{"free", "125.50", "0", "100.00", false},
		{"rounding", "3", "2", "33.33", false},
		{"half up", "32", "31.96", "0.13", false},
		{"large without overflow", "90000000000000000", "72000000000000000", "20.00", false},
		{"zero public", "0", "0", "", true},
		{"negative public", "-100", "20", "", true},
		{"negative sale", "100", "-1", "", true},
		{"above public", "100", "100.01", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			discount, err := DiscountFromSalePrice(MustParse(tc.public), MustParse(tc.sale))
			if tc.invalid {
				if err == nil {
					t.Fatal("invalid prices accepted")
				}
				return
			}
			if err != nil || discount.String() != tc.want {
				t.Fatalf("discount=%s err=%v, want %s", discount, err, tc.want)
			}
		})
	}
}
