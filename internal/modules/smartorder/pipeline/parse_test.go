package pipeline

import (
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
)

// csvOf renders rows as a UTF-8 CSV, which is what Inspect actually reads.
//
// These tests used to call the package's own header detector directly. It no
// longer has one: column detection is productmatch's, the same resolver the
// vendor and catalogue imports run, so the thing worth asserting is what the
// buyer's mapping screen ends up showing. Going through Inspect tests that.
func csvOf(rows [][]string) []byte {
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(strings.Join(r, ","))
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func inspectRows(t *testing.T, rows [][]string) *ParsedFile {
	t.Helper()
	got, err := Inspect(csvOf(rows), "order.csv")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	return got
}

func TestInspectFindsHeaderRowBeneathBanner(t *testing.T) {
	// The layout the spec calls out: a title, a date, a blank, then the headers.
	got := inspectRows(t, [][]string{
		{"صيدلية النور", "", "", ""},
		{"طلب توريد 2026/08", "", "", ""},
		{"", "", "", ""},
		{"اسم الصنف", "كود الصنف", "الباركود", "العدد"},
		{"بانادول", "P1", "6221234567890", "5"},
	})

	if got.HeaderRow != 3 {
		t.Fatalf("expected the header on row 3, got %d", got.HeaderRow)
	}
	if got.Detected[0] != "product_name" {
		t.Errorf("column 0 should be product_name, got %q", got.Detected[0])
	}
	// "العدد" is the alias the spec names explicitly.
	if got.Detected[3] != "quantity" {
		t.Errorf("العدد should map to quantity, got %q", got.Detected[3])
	}
	if got.Confidence["product_name"] <= 0 {
		t.Error("expected a confidence for product_name")
	}
}

func TestInspectHandlesEnglishHeaders(t *testing.T) {
	got := inspectRows(t, [][]string{
		{"Item Name", "SKU", "Barcode", "Qty"},
		{"Panadol Extra", "PAN-24", "6221234567890", "5"},
		{"Congestal", "CON-20", "6221234567891", "3"},
	})

	want := map[int]string{0: "product_name", 1: "sku", 3: "quantity"}
	for col, field := range want {
		if got.Detected[col] != field {
			t.Errorf("column %d: expected %q, got %q", col, field, got.Detected[col])
		}
	}
}

// TestInspectOffersOnlyOrderFields is the guard that replacing the private
// detector did not widen what smart ordering can conclude. A purchase list has
// no prices — the prices are what the run goes and finds — so a column of money
// must not arrive on the mapping screen bound to one.
func TestInspectOffersOnlyOrderFields(t *testing.T) {
	got := inspectRows(t, [][]string{
		{"اسم الصنف", "سعر الجمهور", "الكمية"},
		{"بانادول اكسترا 24 قرص", "48.50", "5"},
		{"كونجستال 20 قرص", "29.00", "3"},
	})

	for col, field := range got.Detected {
		switch field {
		case "product_name", "sku", "barcode", "quantity":
		default:
			t.Errorf("column %d bound to %q, which smart ordering cannot use", col, field)
		}
	}
	if got.Detected[2] != "quantity" {
		t.Errorf("the quantity column should still be found, got %q", got.Detected[2])
	}
}

func TestWeakHeaderGuessesAreNotOffered(t *testing.T) {
	// A confident-looking wrong guess is worse than none: it is the failure the
	// mapping screen exists to catch.
	got := inspectRows(t, [][]string{
		{"ملاحظات", "xyz123", "???"},
		{"a", "b", "c"},
	})
	if field, bound := got.Detected[0]; bound {
		t.Errorf("a notes column must not be offered as %q", field)
	}
}

func TestIsSummaryRowCatchesFooterLabels(t *testing.T) {
	for _, s := range []string{"المجموع", "الإجمالي", "Total", "SUM"} {
		if !isSummaryForTest(s) {
			t.Errorf("%q should be recognised as a footer label, not a product", s)
		}
	}
	for _, s := range []string{"بانادول", "Total Care Shampoo"} {
		if isSummaryForTest(s) {
			t.Errorf("%q is a product name and must not be dropped", s)
		}
	}
}

func TestRawOfKeepsOriginalCells(t *testing.T) {
	// Whatever the mapping decided, the buyer must be able to see what they
	// actually uploaded.
	raw := rawOf([]string{" بانادول ", "", "P1"})
	if raw["0"] != "بانادول" {
		t.Errorf("expected the trimmed original, got %q", raw["0"])
	}
	if _, present := raw["1"]; present {
		t.Error("blank cells should not be stored")
	}
	if raw["2"] != "P1" {
		t.Errorf("expected P1, got %q", raw["2"])
	}
}

func TestCellIsSafeForShortRows(t *testing.T) {
	// A ragged spreadsheet must not panic the importer.
	row := []string{"a"}
	if got := cell(row, 5, true); got != "" {
		t.Errorf("out-of-range column should be empty, got %q", got)
	}
	if got := cell(row, 0, false); got != "" {
		t.Errorf("an unmapped column should be empty, got %q", got)
	}
}

func TestIsRepeatedHeader(t *testing.T) {
	headers := []string{"Item No.", "Item Description", "Preferred Vendor", "Qty"}
	repeated := []string{"Item No.", "Item Description", "Preferred Vendor", "Qty"}
	repeatedCase := []string{"item no.", "item description", "Preferred Vendor", ""}
	dataRow := []string{"10104677", "سيروبايب بلسم للشعر 300 مل", "Parkville", "10"}

	if !isRepeatedHeader(repeated, headers) {
		t.Errorf("exact repeated header row should be detected")
	}
	if !isRepeatedHeader(repeatedCase, headers) {
		t.Errorf("case-insensitive repeated header row should be detected")
	}
	if isRepeatedHeader(dataRow, headers) {
		t.Errorf("data row must not be treated as repeated header")
	}
}

func TestStageWithoutQuantityColumnDefaultsToOne(t *testing.T) {
	content := csvOf([][]string{
		{"اسم الصنف", "كود الصنف"},
		{"بانادول اكسترا", "PAN-01"},
		{"كونجستال اقراص", "CONG-02"},
	})

	m := &smartorder.Mapping{
		HeaderRow: 0,
		Fields: map[int]string{
			0: "product_name",
			1: "sku",
		},
	}

	lines, err := Stage(content, "test.csv", m, 1, 100)
	if err != nil {
		t.Fatalf("unexpected error from Stage: %v", err)
	}

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	for i, l := range lines {
		if l.ImportedQty == nil {
			t.Errorf("line %d: expected ImportedQty to be non-nil", i)
		} else if *l.ImportedQty != 1.0 {
			t.Errorf("line %d: expected ImportedQty to be 1.0, got %g", i, *l.ImportedQty)
		}
	}
}
