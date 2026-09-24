package compare

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

func TestDiscountClampPreventsOverflow(t *testing.T) {
	f := &CompareFile{ID: 1}
	c0, c1, c2 := 0, 1, 2
	f.MappingConfig = MappingConfig{NameCol: &c2, PriceCol: &c0, DiscountCol: &c1}
	s := &Service{}

	// column 1 holds a price, not a percentage - the real shape of the bug
	row := s.extractRowFromRecord([]string{"120.50", "1250.00", "بانادول"}, nil, f, 1)
	if row == nil {
		t.Fatal("row was skipped")
	}
	if row.Discount != 0 {
		t.Errorf("discount = %v, want 0 (out-of-range value must not reach numeric(5,2))", row.Discount)
	}

	// a genuine percentage still survives
	row = s.extractRowFromRecord([]string{"120.50", "12.5", "بانادول"}, nil, f, 2)
	if row.Discount != 12.5 {
		t.Errorf("discount = %v, want 12.5", row.Discount)
	}

	// a barcode read as a price must not overflow numeric(12,2)
	row = s.extractRowFromRecord([]string{"6221048001234567", "10", "بانادول"}, nil, f, 3)
	if row.Price.Minor() >= maxPriceMinor {
		t.Errorf("price minor = %d, want < %d", row.Price.Minor(), maxPriceMinor)
	}
}

func TestIsValidProductRow(t *testing.T) {
	tests := []struct {
		name     string
		rawName  string
		sku      string
		price    int64
		discount float64
		want     bool
	}{
		{"empty name", "", "SKU1", 1000, 10, false},
		{"whitespace name", "   ", "SKU1", 1000, 10, false},
		{"dashes junk", "---", "", 0, 0, false},
		{"dots junk", "...", "SKU2", 500, 0, false},
		{"asterisks junk", "*-*-*", "", 0, 0, false},
		{"slashes junk", "/ / /", "", 0, 0, false},
		{"single character letter", "a", "", 100, 0, false},
		{"single character digit", "1", "", 100, 0, false},
		{"arabic header keyword", "اسم الصنف", "", 0, 0, false},
		{"english header keyword", "Product Name", "", 0, 0, false},
		{"item code keyword", "كود الصنف", "", 0, 0, false},
		{"page divider note", "صفحة 2 من 10", "", 0, 0, false},
		{"total summary line", "Total Summary", "", 0, 0, false},
		{"valid arabic medicine", "بانادول إكسترا", "PAN-01", 3500, 15, true},
		{"valid english medicine", "Augmentin 1g", "AUG-100", 9500, 10, true},
		{"valid short medicine", "AD3", "SKU", 1200, 5, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsValidProductRow(tc.rawName, tc.sku, money.FromMinor(tc.price), tc.discount)
			if got != tc.want {
				t.Errorf("IsValidProductRow(%q) = %v; want %v", tc.rawName, got, tc.want)
			}
		})
	}
}
