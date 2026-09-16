package money

import "testing"

func TestParseAndString(t *testing.T) {
	cases := []struct {
		in    string
		minor int64
		out   string
	}{
		{"0", 0, "0.00"},
		{"0.00", 0, "0.00"},
		{"1", 100, "1.00"},
		{"1.5", 150, "1.50"},
		{"1.05", 105, "1.05"},
		{"1234.56", 123456, "1234.56"},
		{"-0.05", -5, "-0.05"},
		{"-1234.56", -123456, "-1234.56"},
		{".5", 50, "0.50"},
		{"+3.25", 325, "3.25"},
		// The legacy DECIMAL(14,2) ceiling is far below int64 minor units.
		{"99999999999.99", 9999999999999, "99999999999.99"},
	}

	for _, c := range cases {
		got, err := Parse(c.in)
		if err != nil {
			t.Fatalf("Parse(%q) returned error: %v", c.in, err)
		}
		if got.Minor() != c.minor {
			t.Errorf("Parse(%q).Minor() = %d, want %d", c.in, got.Minor(), c.minor)
		}
		if got.String() != c.out {
			t.Errorf("Parse(%q).String() = %q, want %q", c.in, got.String(), c.out)
		}
	}
}

func TestParseRejectsExcessPrecision(t *testing.T) {
	// Rounding here silently is how money goes missing: the caller believes it
	// stored 1.005 and the database holds 1.00 or 1.01 depending on mood.
	if _, err := Parse("1.005"); err == nil {
		t.Fatal("Parse(\"1.005\") should reject more precision than the schema holds")
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", " ", "abc", "1.2.3", "1e5", "1,234.56", "--1", "+"} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) should have failed", in)
		}
	}
}

func TestAddSub(t *testing.T) {
	a, b := MustParse("10.25"), MustParse("4.80")

	sum, err := a.Add(b)
	if err != nil {
		t.Fatal(err)
	}
	if sum.String() != "15.05" {
		t.Errorf("10.25 + 4.80 = %s, want 15.05", sum)
	}

	diff, err := a.Sub(b)
	if err != nil {
		t.Fatal(err)
	}
	if diff.String() != "5.45" {
		t.Errorf("10.25 - 4.80 = %s, want 5.45", diff)
	}
}

func TestMulInt(t *testing.T) {
	// An order line: 12 units at 3.33.
	unit := MustParse("3.33")
	total, err := unit.MulInt(12)
	if err != nil {
		t.Fatal(err)
	}
	if total.String() != "39.96" {
		t.Errorf("3.33 x 12 = %s, want 39.96", total)
	}
}

func TestApplyPercent(t *testing.T) {
	cases := []struct {
		amount string
		bps    int64
		want   string
	}{
		{"100.00", 1250, "12.50"},   // 12.5%
		{"100.00", 10000, "100.00"}, // 100%
		{"33.33", 1000, "3.33"},     // 10% of 33.33 = 3.333 -> 3.33
		{"33.35", 1000, "3.34"},     // 3.335 -> half-up -> 3.34
		{"0.00", 5000, "0.00"},
		{"-20.00", 2500, "-5.00"}, // rounds away from zero on the negative side
	}

	for _, c := range cases {
		got := MustParse(c.amount).ApplyPercent(c.bps)
		if got.String() != c.want {
			t.Errorf("%s at %d bps = %s, want %s", c.amount, c.bps, got, c.want)
		}
	}
}

func TestAllocateLosesNothing(t *testing.T) {
	// The classic case: splitting an order total across vendor shipments where
	// the division is not exact. Independent rounding would produce 3.33 x 3 =
	// 9.99 and lose a piastre.
	total := MustParse("10.00")

	parts, err := total.Allocate([]int64{1, 1, 1})
	if err != nil {
		t.Fatal(err)
	}

	sum := Zero
	for _, p := range parts {
		sum, _ = sum.Add(p)
	}
	if sum.String() != total.String() {
		t.Fatalf("allocation sum = %s, want %s (parts: %v)", sum, total, parts)
	}
	if parts[0].String() != "3.34" || parts[1].String() != "3.33" || parts[2].String() != "3.33" {
		t.Errorf("unexpected distribution: %s %s %s", parts[0], parts[1], parts[2])
	}
}

func TestAllocateWeighted(t *testing.T) {
	total := MustParse("100.00")
	parts, err := total.Allocate([]int64{70, 30})
	if err != nil {
		t.Fatal(err)
	}
	if parts[0].String() != "70.00" || parts[1].String() != "30.00" {
		t.Errorf("weighted allocation = %s / %s, want 70.00 / 30.00", parts[0], parts[1])
	}
}

func TestAllocateNegative(t *testing.T) {
	// Refunds allocate too, and the remainder must move in the right direction.
	total := MustParse("-10.00")
	parts, err := total.Allocate([]int64{1, 1, 1})
	if err != nil {
		t.Fatal(err)
	}
	sum := Zero
	for _, p := range parts {
		sum, _ = sum.Add(p)
	}
	if sum.String() != "-10.00" {
		t.Fatalf("negative allocation sum = %s, want -10.00", sum)
	}
}

func TestJSONRoundTripIsAString(t *testing.T) {
	a := MustParse("1234.56")

	raw, err := a.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	// A JSON number would be parsed as a float by most clients, reintroducing
	// exactly the imprecision this package exists to prevent.
	if string(raw) != `"1234.56"` {
		t.Fatalf("MarshalJSON = %s, want a quoted string", raw)
	}

	var back Amount
	if err := back.UnmarshalJSON(raw); err != nil {
		t.Fatal(err)
	}
	if back.Minor() != a.Minor() {
		t.Errorf("round trip changed value: %s -> %s", a, back)
	}
}

func TestScanFromDatabaseText(t *testing.T) {
	var a Amount
	if err := a.Scan("1234.56"); err != nil {
		t.Fatal(err)
	}
	if a.String() != "1234.56" {
		t.Errorf("Scan gave %s, want 1234.56", a)
	}

	var b Amount
	if err := b.Scan([]byte("0.07")); err != nil {
		t.Fatal(err)
	}
	if b.Minor() != 7 {
		t.Errorf("Scan([]byte) gave %d minor units, want 7", b.Minor())
	}
}

func TestFormatWithCommas(t *testing.T) {
	tests := []struct {
		amount   Amount
		expected string
	}{
		{MustParse("0.00"), "0.00"},
		{MustParse("5.50"), "5.50"},
		{MustParse("100.00"), "100.00"},
		{MustParse("1234.56"), "1,234.56"},
		{MustParse("12345.67"), "12,345.67"},
		{MustParse("123456.78"), "123,456.78"},
		{MustParse("1234567.89"), "1,234,567.89"},
		{MustParse("-1234.50"), "-1,234.50"},
		{MustParse("-1234567.00"), "-1,234,567.00"},
	}

	for _, tt := range tests {
		got := tt.amount.FormatWithCommas()
		if got != tt.expected {
			t.Errorf("FormatWithCommas(%s) = %q, want %q", tt.amount, got, tt.expected)
		}
	}

	if got := FormatIntWithCommas(0); got != "0" {
		t.Errorf("FormatIntWithCommas(0) = %q, want 0", got)
	}
	if got := FormatIntWithCommas(999); got != "999" {
		t.Errorf("FormatIntWithCommas(999) = %q, want 999", got)
	}
	if got := FormatIntWithCommas(1000); got != "1,000" {
		t.Errorf("FormatIntWithCommas(1000) = %q, want 1,000", got)
	}
	if got := FormatIntWithCommas(1234567); got != "1,234,567" {
		t.Errorf("FormatIntWithCommas(1234567) = %q, want 1,234,567", got)
	}
	if got := FormatIntWithCommas(-5000); got != "-5,000" {
		t.Errorf("FormatIntWithCommas(-5000) = %q, want -5,000", got)
	}
}

