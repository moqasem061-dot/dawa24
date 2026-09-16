package catalog_test

import (
	"errors"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

func TestCoerceDecimalReadsRealWorldPrices(t *testing.T) {
	// Every one of these arrived as a price in a real supplier file. money.Parse
	// refuses all but the last two, and the old importer swallowed the error and
	// stored a zero — a whole catalogue imported free of charge.
	tests := []struct {
		in      string
		want    string
		percent bool
		rounded bool
	}{
		{"115.00", "115.00", false, false},
		{"1,234.50", "1234.50", false, false}, // Anglo grouping
		{"1.234,50", "1234.50", false, false}, // European grouping
		{"1.234.567,89", "1234567.89", false, false},
		{"٤٢٫٥٠", "42.50", false, false},       // Arabic-Indic digits and separator
		{"115.00 ج.م", "115.00", false, false}, // currency suffix
		{"42 EGP", "42", false, false},
		{"  35  ", "35", false, false},
		{"20%", "20", true, false},
		{"(12.50)", "-12.50", false, false}, // accounting negative
		{"-15", "-15", false, false},
		{"0000115", "115", false, false}, // zero-padded export
		{"25.005", "25.01", false, true}, // more precision than NUMERIC(12,2)
		{"1,234", "1234", false, false},  // three-digit tail is grouping
		{"1,23", "1.23", false, false},   // two-digit tail is a decimal
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := catalog.CoerceDecimal(tc.in)
			if err != nil {
				t.Fatalf("CoerceDecimal(%q) failed: %v", tc.in, err)
			}
			if got.Canonical != tc.want {
				t.Errorf("canonical = %q, want %q", got.Canonical, tc.want)
			}
			if got.Percent != tc.percent {
				t.Errorf("percent = %v, want %v", got.Percent, tc.percent)
			}
			if got.Rounded != tc.rounded {
				t.Errorf("rounded = %v, want %v", got.Rounded, tc.rounded)
			}
		})
	}
}

func TestCoerceDecimalSeparatesAbsentFromUnreadable(t *testing.T) {
	// The two need different messages: "the supplier left the price out" is
	// normal, "the supplier typed something we cannot read" needs attention.
	absent := []string{"", "  ", "-", "N/A", "#N/A", "#DIV/0!", "لا يوجد"}
	for _, in := range absent {
		if _, err := catalog.CoerceDecimal(in); !errors.Is(err, catalog.ErrNoValue) {
			t.Errorf("CoerceDecimal(%q) = %v, want ErrNoValue", in, err)
		}
	}

	unreadable := []string{"راجع الادارة", "abc", "سعر خاص", "??"}
	for _, in := range unreadable {
		if _, err := catalog.CoerceDecimal(in); !errors.Is(err, catalog.ErrNotNumeric) {
			t.Errorf("CoerceDecimal(%q) = %v, want ErrNotNumeric", in, err)
		}
	}
}

func TestCoerceDecimalCarriesOnRounding(t *testing.T) {
	// .995 rounds up through the fraction and into the integer part.
	got, err := catalog.CoerceDecimal("9.995")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if got.Canonical != "10.00" {
		t.Errorf("canonical = %q, want 10.00", got.Canonical)
	}
}

func TestCoerceMoneyProducesStorableAmounts(t *testing.T) {
	amt, _, err := catalog.CoerceMoney("1,234.50")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if amt.String() != "1234.50" {
		t.Errorf("amount = %s, want 1234.50", amt.String())
	}
}

func TestCoerceStatusMapsToAllowedValues(t *testing.T) {
	// The products table has a CHECK on status. An unrecognised value must be
	// reported, never written: a rejected CHECK aborts the whole transaction.
	tests := map[string]catalog.ProductStatus{
		"active": catalog.StatusActive,
		"نشط":    catalog.StatusActive,
		"مفعل":   catalog.StatusActive,
		"معطل":   catalog.StatusInactive,
		"مرفوض":  catalog.StatusRejected,

		// The catalogue has no review queue. A supplier's file describing its
		// own workflow as "pending" is not an instruction to park the row where
		// no matching engine can see it — an administrator importing a product
		// is the act that approves it.
		"pending":      catalog.StatusActive,
		"قيد المراجعة": catalog.StatusActive,
		"قيد التدقيق":  catalog.StatusActive,
	}
	for in, want := range tests {
		got, ok := catalog.CoerceStatus(in)
		if !ok || got != want {
			t.Errorf("CoerceStatus(%q) = %q,%v; want %q,true", in, got, ok, want)
		}
	}

	// Nothing an import can read may produce a pending product.
	for _, in := range []string{"pending", "review", "معلق", "قيد المراجعة", "قيد التدقيق", "منتظر"} {
		if got, _ := catalog.CoerceStatus(in); got == catalog.StatusPending {
			t.Errorf("CoerceStatus(%q) still produces a pending product", in)
		}
	}

	if _, ok := catalog.CoerceStatus("حالة غريبة"); ok {
		t.Error("an unrecognised status was accepted")
	}
}

func TestCoerceIntReadsQuantities(t *testing.T) {
	tests := map[string]int64{"45": 45, "45.00": 45, "45 علبة": 45, "١٢٠": 120, "1,200": 1200}
	for in, want := range tests {
		got, err := catalog.CoerceInt(in)
		if err != nil {
			t.Errorf("CoerceInt(%q) failed: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("CoerceInt(%q) = %d, want %d", in, got, want)
		}
	}
}
