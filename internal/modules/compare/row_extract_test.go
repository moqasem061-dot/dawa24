package compare

import "testing"

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
