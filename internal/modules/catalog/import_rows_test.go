package catalog_test

import (
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// parseCSV is the shortest path from a literal file to a parse result.
func parseCSV(t *testing.T, content string) *catalog.ParseResult {
	t.Helper()
	data, err := catalog.ReadSpreadsheet([]byte(content), "test.csv")
	if err != nil {
		t.Fatalf("ReadSpreadsheet failed: %v", err)
	}
	return catalog.ParseProducts(data)
}

func TestParseProductsSkipsTitleRowsAndFindsHeader(t *testing.T) {
	// Distributor exports lead with a title and a blank line before the real
	// header. Treating row 1 as the header maps every field wrongly.
	res := parseCSV(t, strings.Join([]string{
		"قائمة أصناف شركة النور - صادر بتاريخ 2026/08/01",
		",,,",
		"اسم الصنف,كود الصنف,سعر البيع,الشركة المصنعة",
		"بانادول اكسترا,PAN-1,25.00,جلاكسو",
		"كتافلام,PAN-2,42.00,نوفارتس",
	}, "\n"))

	if res.Stats.HeaderRow != 2 {
		t.Fatalf("header detected at row index %d, want 2", res.Stats.HeaderRow)
	}
	if len(res.Products) != 2 {
		t.Fatalf("got %d products, want 2", len(res.Products))
	}
	if res.Products[0].Price.String() != "25.00" {
		t.Errorf("price = %s, want 25.00", res.Products[0].Price.String())
	}
}

func TestParseProductsFiltersRepeatedPrintHeaders(t *testing.T) {
	// Paginated exports reprint the column titles every page — one real file
	// carried 114 of them. Matching against the detected header generalises past
	// the previous hardcoded list of nine literal strings.
	res := parseCSV(t, strings.Join([]string{
		"اسم الصنف,كود الصنف,سعر البيع",
		"بانادول,PAN-1,25.00",
		"اسم الصنف,كود الصنف,سعر البيع",
		"كتافلام,PAN-2,42.00",
		"اسم الصنف,كود الصنف,سعر البيع",
		"اوجمنتين,PAN-3,115.00",
	}, "\n"))

	if res.Stats.RepeatedHeader != 2 {
		t.Errorf("filtered %d repeated headers, want 2", res.Stats.RepeatedHeader)
	}
	if len(res.Products) != 3 {
		t.Errorf("got %d products, want 3", len(res.Products))
	}
}

func TestParseProductsRejectsBadPricesWithRowNumbers(t *testing.T) {
	// A row number is the whole point of the report: it must match the gutter in
	// the admin's copy of Excel so they can go straight to the cell.
	res := parseCSV(t, strings.Join([]string{
		"اسم الصنف,كود الصنف,السعر",
		"بانادول,PAN-1,25.00",
		"صنف بسعر سالب,PAN-2,-15.00",
		"صنف بسعر نصي,PAN-3,راجع الادارة",
		"كتافلام,PAN-4,42.00",
	}, "\n"))

	if len(res.Products) != 2 {
		t.Fatalf("got %d products, want 2", len(res.Products))
	}
	if res.Stats.RejectedRows != 2 {
		t.Errorf("rejected %d rows, want 2", res.Stats.RejectedRows)
	}

	rejectedRows := map[int]bool{}
	for _, issue := range res.Issues {
		if issue.Severity == catalog.SeverityError {
			rejectedRows[issue.Row] = true
		}
	}
	for _, want := range []int{3, 4} {
		if !rejectedRows[want] {
			t.Errorf("no error reported against spreadsheet row %d", want)
		}
	}
}

func TestParseProductsRejectsPriceBeyondColumnRange(t *testing.T) {
	// catalog.products stores NUMERIC(12,2). A larger value would be refused by
	// PostgreSQL and abort the whole transaction, losing an otherwise clean
	// import of thousands of rows for one careless cell.
	res := parseCSV(t, "اسم الصنف,السعر\nصنف فلكي,99999999999999.99\nصنف عادي,50.00\n")

	if len(res.Products) != 1 {
		t.Fatalf("got %d products, want 1", len(res.Products))
	}
	if res.Stats.RejectedRows != 1 {
		t.Errorf("rejected %d rows, want 1", res.Stats.RejectedRows)
	}
}

func TestParseProductsMergesDuplicatesWithinFile(t *testing.T) {
	// A supplier listing the same SKU twice — once with a price, once with a
	// barcode — should end with one complete product, not two half-empty ones.
	res := parseCSV(t, strings.Join([]string{
		"اسم الصنف,كود الصنف,الباركود,السعر",
		"بانادول اكسترا,PAN-1,,25.00",
		"بانادول اكسترا,PAN-1,6221234567890,",
		"كتافلام,PAN-2,,42.00",
	}, "\n"))

	if len(res.Products) != 2 {
		t.Fatalf("got %d products, want 2", len(res.Products))
	}
	if res.Stats.DuplicateRows != 1 {
		t.Errorf("merged %d duplicates, want 1", res.Stats.DuplicateRows)
	}

	merged := res.Products[0]
	if merged.Price.String() != "25.00" {
		t.Errorf("merged price = %s, want 25.00 (the later blank must not erase it)", merged.Price.String())
	}
	if merged.Barcode != "6221234567890" {
		t.Errorf("merged barcode = %q, want the value the second row supplied", merged.Barcode)
	}
}

func TestParseProductsDeduplicatesAcrossArabicSpellings(t *testing.T) {
	// Two rows for the same product, differing only by hamza and ta-marbuta.
	// Left alone they become two catalogue entries a pharmacist has to reconcile.
	res := parseCSV(t, strings.Join([]string{
		"اسم الصنف,السعر",
		"أوجمنتين حقنة,115.00",
		"اوجمنتين حقنه,115.00",
	}, "\n"))

	if len(res.Products) != 1 {
		t.Fatalf("got %d products, want 1 (spelling variants of one name)", len(res.Products))
	}
}

func TestParseProductsKeepsEnglishNameOutOfArabic(t *testing.T) {
	// The old importer copied the Arabic name into the English slot whenever the
	// file had no English column, which made the catalogue look translated when
	// it was not and made every English search match Arabic text.
	res := parseCSV(t, "اسم الصنف,السعر\nبانادول اكسترا,25.00\n")

	if len(res.Products) != 1 {
		t.Fatalf("got %d products, want 1", len(res.Products))
	}
	if _, hasEN := res.Products[0].Name[i18n.EN]; hasEN {
		t.Error("an English name was invented for a file that carried none")
	}
}

func TestParseProductsReadsDiscountAsPercentAndAmount(t *testing.T) {
	res := parseCSV(t, strings.Join([]string{
		"اسم الصنف,السعر,الخصم",
		"صنف بنسبة,100.00,20%",
		"صنف بمبلغ,100.00,15.00",
		"خصم أكبر من السعر,100.00,150.00",
	}, "\n"))

	if len(res.Products) != 3 {
		t.Fatalf("got %d products, want 3", len(res.Products))
	}
	if got := res.Products[0].Discount.String(); got != "20.00" {
		t.Errorf("percent discount = %s, want 20.00 (20%% of 100)", got)
	}
	if got := res.Products[1].Discount.String(); got != "15.00" {
		t.Errorf("amount discount = %s, want 15.00", got)
	}
	if got := res.Products[2].Discount.String(); got != "0.00" {
		t.Errorf("discount exceeding the price = %s, want 0.00 with a warning", got)
	}
}

func TestParseProductsFallsBackToPublicPrice(t *testing.T) {
	// A master catalogue file often carries only the public price. Storing zero
	// because the column was not literally named "السعر" prices the whole
	// catalogue at nothing.
	res := parseCSV(t, "اسم الصنف,سعر الجمهور\nبانادول,55.00\n")

	if len(res.Products) != 1 {
		t.Fatalf("got %d products, want 1", len(res.Products))
	}
	if got := res.Products[0].Price.String(); got != "55.00" {
		t.Errorf("price = %s, want 55.00 from the public price column", got)
	}
}

func TestParseProductsWarnsWhenNoHeaderFound(t *testing.T) {
	// Reading a headerless file by column order is a guess. It is worth making,
	// because these files exist, but the admin must be told it was a guess.
	res := parseCSV(t, "10202898,بوباى صن سكرين كريم 50 جم,Parkville\n10106853,بريث داى غسول فم,Parkville\n")

	if !res.Plan.Positional {
		t.Error("expected the plan to be flagged positional")
	}
	if len(res.Products) != 2 {
		t.Fatalf("got %d products, want 2", len(res.Products))
	}
	if res.Stats.Warnings == 0 {
		t.Error("no warning raised for a headerless file")
	}
}

func TestParseProductsKeepsRowsThatHaveOnlyAnIdentifier(t *testing.T) {
	// Dropping these loses stock the pharmacy owns. They are imported under a
	// placeholder name and flagged for review instead.
	res := parseCSV(t, "اسم الصنف,كود الصنف,السعر\n,PAN-5,20.00\nبانادول,PAN-6,25.00\n")

	if len(res.Products) != 2 {
		t.Fatalf("got %d products, want 2", len(res.Products))
	}
	if !strings.Contains(res.Products[0].Name.Get(i18n.AR), "PAN-5") {
		t.Errorf("placeholder name = %q, want one built from the code", res.Products[0].Name.Get(i18n.AR))
	}
}

func TestParseProductsInfersDosageAndConcentration(t *testing.T) {
	res := parseCSV(t, "اسم الصنف,السعر\nبوباى صن سكرين كريم 50 جم (+50SPF),25.00\nكتافلام فوار 50 مجم,42.00\n")

	if len(res.Products) != 2 {
		t.Fatalf("got %d products, want 2", len(res.Products))
	}
	if res.Products[0].DosageForm != "كريم" {
		t.Errorf("dosage form = %q, want كريم", res.Products[0].DosageForm)
	}
	if res.Products[0].Concentration != "50 جم" {
		t.Errorf("concentration = %q, want '50 جم'", res.Products[0].Concentration)
	}
	if res.Products[1].DosageForm != "أكياس فوار" {
		t.Errorf("dosage form = %q, want 'أكياس فوار'", res.Products[1].DosageForm)
	}
}

func TestParseProductsCrossFillsSKUAndBarcode(t *testing.T) {
	// A file with only a SKU fills an empty barcode from it.
	res := parseCSV(t, "اسم الصنف,كود الصنف\nبانادول,PAN-123\n")

	if len(res.Products) != 1 {
		t.Fatalf("got %d products, want 1", len(res.Products))
	}
	if res.Products[0].Barcode != "PAN-123" {
		t.Errorf("barcode = %q, want the SKU carried across", res.Products[0].Barcode)
	}
}

func TestParseProductsReportsMissingColumns(t *testing.T) {
	res := parseCSV(t, "اسم الصنف\nبانادول\nكتافلام\n")

	if len(res.MissingFields) == 0 {
		t.Fatal("no missing columns reported for a name-only file")
	}
	found := false
	for _, label := range res.MissingFields {
		if label == catalog.FieldLabels[catalog.FieldPrice] {
			found = true
		}
	}
	if !found {
		t.Errorf("missing fields %v do not include the price column", res.MissingFields)
	}
}
