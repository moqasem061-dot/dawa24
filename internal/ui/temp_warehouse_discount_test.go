package ui

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
)

func TestWarehouseDiscountUsesTheSameLiteralParser(t *testing.T) {
	file := &compare.CompareFile{ID: 1}
	rows := buildTempWarehouseRows(file, [][]string{
		{"1", "Panadol Extra 24 tablets", "100", "0.25"},
		{"2", "Panadol Extra 24 tablets", "100", "٢٥٫٣٥٪"},
		{"3", "Panadol Extra 24 tablets", "100", "1250"},
	}, 1, 0, 1, 2, 3)
	if len(rows) != 3 {
		t.Fatalf("rows = %d", len(rows))
	}
	for i, want := range []float64{0.25, 25.35, 0} {
		if rows[i].Discount != want {
			t.Fatalf("row %d discount = %v, want %v", i, rows[i].Discount, want)
		}
	}
	if rows[0].PriceAfterDiscount.String() != "99.75" || rows[1].PriceAfterDiscount.String() != "74.65" {
		t.Fatalf("incorrect exact prices: %s, %s", rows[0].PriceAfterDiscount, rows[1].PriceAfterDiscount)
	}
}
