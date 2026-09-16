package catalog_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

// paginatedExport builds the shape the real distributor file has: a title line,
// then the column titles reprinted every `every` rows with a blank separator.
func paginatedExport(products, every int) string {
	const header = "Item No.,Item Description,Preferred Vendor"
	rows := []string{"قائمة الأصناف - تصدير", header}
	for i := 0; i < products; i++ {
		if i > 0 && i%every == 0 {
			rows = append(rows, "", header)
		}
		rows = append(rows, fmt.Sprintf("10%06d,صنف دوائي رقم %d كريم,Parkville", i, i))
	}
	return strings.Join(rows, "\n") + "\n"
}

func TestAnalyzeLayoutFindsEveryBlockInAPaginatedExport(t *testing.T) {
	// The file this was built against reprints its header 114 times. An importer
	// that stops at the first reprint, or imports the reprints as products,
	// loses or corrupts most of a nine-thousand-row catalogue.
	res := parseCSV(t, paginatedExport(900, 79))

	if len(res.Products) != 900 {
		t.Fatalf("parsed %d products, want 900 — every block must be read", len(res.Products))
	}
	wantBlocks := 900/79 + 1
	if got := len(res.Layout.Blocks); got != wantBlocks {
		t.Errorf("found %d blocks, want %d", got, wantBlocks)
	}
	if res.Stats.RepeatedHeader != wantBlocks-1 {
		t.Errorf("counted %d repeated headers, want %d", res.Stats.RepeatedHeader, wantBlocks-1)
	}
}

// Every row of the file must end up in exactly one bucket. Without this, an
// importer can silently drop rows and still look healthy.
func TestParseProductsAccountsForEveryRow(t *testing.T) {
	res := parseCSV(t, paginatedExport(300, 50))

	// The title row and the primary header are structure, not data.
	const structuralRows = 2
	accounted := len(res.Products) + res.Stats.RejectedRows + res.Stats.DuplicateRows +
		res.Stats.RepeatedHeader + res.Stats.EmptyRows + structuralRows

	if accounted != res.Stats.TotalRowsRead {
		t.Fatalf("accounted for %d rows of %d — %d unexplained",
			accounted, res.Stats.TotalRowsRead, res.Stats.TotalRowsRead-accounted)
	}
}

func TestAnalyzeLayoutReadsSectionsWithDifferentColumns(t *testing.T) {
	// Two tables stacked in one sheet, the second adding a price column. Forcing
	// the second through the first's mapping would read its prices as vendors.
	res := parseCSV(t, strings.Join([]string{
		"اسم الصنف,كود الصنف,الشركة المصنعة",
		"بانادول اكسترا,PAN-1,جلاكسو",
		"",
		"اسم الصنف,كود الصنف,سعر البيع",
		"كتافلام أقراص,PAN-2,42.00",
	}, "\n"))

	if len(res.Products) != 2 {
		t.Fatalf("parsed %d products, want 2", len(res.Products))
	}
	if res.Layout.VariantBlocks != 1 {
		t.Errorf("VariantBlocks = %d, want 1", res.Layout.VariantBlocks)
	}
	if got := res.Products[0].ManufacturingCompanies; got != "جلاكسو" {
		t.Errorf("first block manufacturer = %q, want جلاكسو", got)
	}
	if got := res.Products[1].Price.String(); got != "42.00" {
		t.Errorf("second block price = %s, want 42.00 — its own header must be used", got)
	}
}

func TestAnalyzeLayoutFindsTableBelowPreamble(t *testing.T) {
	// A table that starts at row 6 under a block of report metadata. Assuming
	// row 1 is the header maps every field wrongly.
	res := parseCSV(t, strings.Join([]string{
		"شركة النور للتوزيع الدوائي",
		"العنوان: القاهرة - مصر الجديدة",
		"تليفون: 0223456789",
		"تاريخ التصدير: 2026/08/01",
		"",
		"اسم الصنف,كود الصنف,سعر البيع",
		"بانادول اكسترا,PAN-1,25.00",
		"كتافلام أقراص,PAN-2,42.00",
	}, "\n"))

	if res.Stats.HeaderRow != 5 {
		t.Fatalf("header row = %d, want 5", res.Stats.HeaderRow)
	}
	if len(res.Products) != 2 {
		t.Fatalf("parsed %d products, want 2", len(res.Products))
	}
	if res.Products[0].Price.String() != "25.00" {
		t.Errorf("price = %s, want 25.00", res.Products[0].Price.String())
	}
}

func TestLayoutOverridesForceHeaderRow(t *testing.T) {
	// The admin's judgement wins over the analysis. Here they point at a row the
	// detector would not have chosen.
	content := strings.Join([]string{
		"اسم الصنف,كود الصنف,سعر البيع",
		"عنوان فرعي,لا شيء,لا شيء",
		"الصنف,الكود,الثمن",
		"بانادول اكسترا,PAN-1,25.00",
	}, "\n")

	data, err := catalog.ReadSpreadsheet([]byte(content), "f.csv")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	// Row 3 as the admin counts it, one-based.
	res := catalog.ParseProductsWithOverrides(data, catalog.LayoutOverrides{HeaderRow: 3})
	if res.Stats.HeaderRow != 2 {
		t.Fatalf("header row = %d, want 2 (zero-based for the forced row 3)", res.Stats.HeaderRow)
	}
	if len(res.Products) != 1 {
		t.Fatalf("parsed %d products, want 1", len(res.Products))
	}
	if got := res.Products[0].Name.Get("ar"); got != "بانادول اكسترا" {
		t.Errorf("name = %q, want بانادول اكسترا", got)
	}
}

func TestLayoutOverridesRebindAndIgnoreColumns(t *testing.T) {
	content := "اسم الصنف,كود الصنف,سعر البيع\nبانادول اكسترا,PAN-1,25.00\n"
	data, err := catalog.ReadSpreadsheet([]byte(content), "f.csv")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	res := catalog.ParseProductsWithOverrides(data, catalog.LayoutOverrides{
		Columns: map[string]int{
			// Read column 3 as the public price instead of the selling price,
			// and stop reading the code column at all.
			catalog.FieldPublicPrice: 3,
			catalog.FieldPrice:       catalog.IgnoreColumn,
			catalog.FieldSKU:         catalog.IgnoreColumn,
		},
	})

	if len(res.Products) != 1 {
		t.Fatalf("parsed %d products, want 1", len(res.Products))
	}
	got := res.Products[0]
	if got.OldPrice.String() != "25.00" {
		t.Errorf("public price = %s, want 25.00", got.OldPrice.String())
	}
	if got.SKU != "" {
		t.Errorf("sku = %q, want empty — the column was unbound", got.SKU)
	}
}

func TestLayoutOverridesClipRowRange(t *testing.T) {
	content := strings.Join([]string{
		"اسم الصنف,كود الصنف,سعر البيع",
		"بانادول اكسترا,PAN-1,25.00",
		"كتافلام أقراص,PAN-2,42.00",
		"اوجمنتين,PAN-3,115.00",
		"إجمالي,,182.00",
	}, "\n")

	data, err := catalog.ReadSpreadsheet([]byte(content), "f.csv")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	// Stop before the totals line, which is not a product.
	res := catalog.ParseProductsWithOverrides(data, catalog.LayoutOverrides{LastDataRow: 4})
	if len(res.Products) != 3 {
		t.Fatalf("parsed %d products, want 3 — the totals row must be excluded", len(res.Products))
	}
}
