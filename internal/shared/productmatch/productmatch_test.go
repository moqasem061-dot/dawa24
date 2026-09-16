package productmatch

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// analyse is the whole read of a fixture: bytes in, resolved mapping out.
func analyse(t *testing.T, csv string) *Analysis {
	t.Helper()
	book, err := sheet.Open([]byte(csv), "fixture.csv")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	t.Cleanup(func() { _ = book.Close() })

	a, err := Analyze(book, NewVocabulary(nil, nil, nil, nil))
	if err != nil {
		t.Fatalf("analyze fixture: %v", err)
	}
	return a
}

// bound asserts which column a field resolved to, one-based as a vendor counts.
func bound(t *testing.T, a *Analysis, field Field, wantColumn int) {
	t.Helper()
	col, ok := a.Mapping.Column(field)
	if !ok {
		t.Fatalf("%s was not bound to any column", field)
	}
	if col+1 != wantColumn {
		t.Fatalf("%s bound to column %d, want %d", field, col+1, wantColumn)
	}
}

// priceList builds a fixture with the given header and a spread of realistic
// Egyptian rows: public prices from 22 to 340, discounts from 24 to 42.
func priceList(header string) string {
	rows := [][4]string{
		{"2702", "ليناجليفلوزين 5-10مجم 30قرص", "342", "27"},
		{"8716", "لينكس 14 كبسولة كبار", "260", "24"},
		{"3132", "ماكسى كير كريم", "137", "31"},
		{"4337", "موفكسير 50-500 مجم 20 قرص", "78", "35"},
		{"1784", "موفينتور ادفانس 20 قرص", "530", "35"},
		{"5724", "نوكال 25 كيس", "30", "42"},
		{"5713", "نوكال 250 جم", "135", "42"},
		{"1067", "استربسلز عسل وليمون", "170", "26"},
		{"2432", "اربتاج 200 مجم 30 قرص", "75", "33"},
		{"2486", "ارتيكافول 3 امبول", "33", "32"},
	}
	out := header + "\n"
	// Thirty rows: enough for the percentile-based tests to be meaningful.
	for i := 0; i < 3; i++ {
		for _, r := range rows {
			out += r[0] + "," + r[1] + "," + r[2] + "," + r[3] + "\n"
		}
	}
	return out
}

func TestHeaderNamesResolveTheObviousFile(t *testing.T) {
	a := analyse(t, priceList("كود الصنف,اسم الصنف,سعر الجمهور,الخصم"))
	bound(t, a, FieldSKU, 1)
	bound(t, a, FieldName, 2)
	bound(t, a, FieldPublicPrice, 3)
	bound(t, a, FieldDiscountPct, 4)
}

// The headers below label a discount column in real distributor files and none
// of them means "discount". Only the values can settle it.
func TestValuesResolveAMisleadingDiscountHeader(t *testing.T) {
	for _, header := range []string{"القائمة", "المرجح", "جملة", "مندوب", "ج الجملة"} {
		t.Run(header, func(t *testing.T) {
			a := analyse(t, priceList("الكود,الصنف,سعر ج,"+header))
			bound(t, a, FieldDiscountPct, 4)
			bound(t, a, FieldPublicPrice, 3)
		})
	}
}

// A file whose columns are in an unusual order must still resolve, because the
// assignment is global rather than left to right.
func TestColumnOrderDoesNotMatter(t *testing.T) {
	rows := "الخصم,سعر ج,الكود,الصنف\n"
	for i := 0; i < 30; i++ {
		rows += "32,45,1794,ابل لايت 30قرص\n"
	}
	a := analyse(t, rows)
	bound(t, a, FieldDiscountPct, 1)
	bound(t, a, FieldPublicPrice, 2)
	bound(t, a, FieldSKU, 3)
	bound(t, a, FieldName, 4)
}

// "الباركود الدولي" contains "كود", which is how a left-to-right mapper binds it
// to the item code and then never finds the real one.
func TestBarcodeDoesNotStealTheItemCodeColumn(t *testing.T) {
	rows := "اسم الصنف,الباركود الدولي,كود الصنف,سعر الجمهور\n"
	for i := 0; i < 30; i++ {
		rows += "بانادول إكسترا 24 قرص,6221234567890,PAN-24,45.00\n"
	}
	a := analyse(t, rows)
	bound(t, a, FieldBarcode, 2)
	bound(t, a, FieldSKU, 3)
}

// A title line and a run of blanks above the header is how half of the real
// files arrive.
func TestHeaderBelowATitleBlock(t *testing.T) {
	rows := "أرصده المخازن 02/08/2026,,\n,,\n,,\nإسم الصنف,السعر,خصم اساسى\n"
	for i := 0; i < 30; i++ {
		rows += "ابياموكس400مجم شراب,70,32\n"
	}
	a := analyse(t, rows)
	if a.Layout.HeaderRow != 3 {
		t.Fatalf("header row %d, want 3", a.Layout.HeaderRow)
	}
	bound(t, a, FieldName, 1)
	bound(t, a, FieldPublicPrice, 2)
	bound(t, a, FieldDiscountPct, 3)
}

// Three columns satisfying base × (1 − rate/100) = net are those three fields,
// whatever their headers claim.
func TestArithmeticRelationOverridesHeaders(t *testing.T) {
	rows := "أ,ب,ج,الصنف\n"
	for i := 0; i < 40; i++ {
		// 100 at 20% is 80, every row.
		rows += "100,20,80,صنف تجريبي رقم واحد\n"
	}
	a := analyse(t, rows)
	if a.Relation == nil {
		t.Fatal("no arithmetic relation found between the price columns")
	}
	bound(t, a, FieldPublicPrice, 1)
	bound(t, a, FieldDiscountPct, 2)
	bound(t, a, FieldNetPrice, 3)
}

func TestRowReadingReconcilesPrices(t *testing.T) {
	a := analyse(t, priceList("كود الصنف,اسم الصنف,سعر الجمهور,الخصم"))
	reader := NewReader(a.Mapping, DefaultParseOptions())

	row, ok := reader.Read(2, []string{"2702", "ليناجليفلوزين 5-10مجم 30قرص", "342", "27"})
	if !ok {
		t.Fatalf("row rejected: %+v", row.Issues)
	}
	if got := row.PublicPrice.String(); got != "342.00" {
		t.Fatalf("public price %s, want 342.00", got)
	}
	if row.DiscountBps != 2700 {
		t.Fatalf("discount %d bps, want 2700", row.DiscountBps)
	}
	// 342 less 27 percent is 249.66 exactly.
	if got := row.NetPrice.String(); got != "249.66" {
		t.Fatalf("net price %s, want 249.66", got)
	}
}

func TestImpossibleDiscountIsRefusedNotApplied(t *testing.T) {
	a := analyse(t, priceList("كود الصنف,اسم الصنف,سعر الجمهور,الخصم"))
	reader := NewReader(a.Mapping, DefaultParseOptions())

	row, ok := reader.Read(9, []string{"2702", "صنف به خطأ مطبعي", "342", "46223"})
	if !ok {
		t.Fatal("row should survive a bad discount cell")
	}
	if row.DiscountBps != 0 {
		t.Fatalf("discount %d bps applied, want it ignored", row.DiscountBps)
	}
	if len(row.Issues) == 0 {
		t.Fatal("an impossible discount must be reported, not silently dropped")
	}
}

// One real file carries the name in the code column on about one row in eight.
func TestSwappedNameAndCodeAreRepaired(t *testing.T) {
	rows := "كود الصنف,اسم الصنف,سعر الجمهور,الخصم\n"
	for i := 0; i < 30; i++ {
		rows += "2975,ابل لايت 30 قرص,45,32\n"
	}
	a := analyse(t, rows)
	reader := NewReader(a.Mapping, DefaultParseOptions())

	row, ok := reader.Read(5, []string{"هاى بيوتك ان شراب 600 مجم", "23169", "321", "9"})
	if !ok {
		t.Fatalf("row rejected: %+v", row.Issues)
	}
	if row.Name != "هاى بيوتك ان شراب 600 مجم" {
		t.Fatalf("name %q, want the swapped-back product name", row.Name)
	}
	if row.SKU != "23169" {
		t.Fatalf("sku %q, want 23169", row.SKU)
	}
}

func TestMatchingSeparatesTwoStrengthsOfOneBrand(t *testing.T) {
	idx := NewIndex([]MasterProduct{
		{ID: 1, NameAR: "أوجمنتين 1 جم 14 قرص", Concentration: "1g"},
		{ID: 2, NameAR: "أوجمنتين 625 مجم 20 قرص", Concentration: "625mg"},
	})
	res := idx.Match(&Row{Name: "اوجمنتين 625مجم 20 قرص"}, DefaultMatchOptions())
	if res.ProductID != 2 {
		t.Fatalf("matched product %d (%s), want 2", res.ProductID, res.Level)
	}
}

func TestMatchingSeparatesTwoPackSizes(t *testing.T) {
	idx := NewIndex([]MasterProduct{
		{ID: 1, NameAR: "ستار فيل مناديل ميسيلار 25 منديل"},
		{ID: 2, NameAR: "ستار فيل مناديل ميسيلار 50 منديل"},
	})
	res := idx.Match(&Row{Name: "ستار فيل مناديل ميسيلار 50 منديل"}, DefaultMatchOptions())
	if res.ProductID != 2 {
		t.Fatalf("matched product %d (%s), want 2", res.ProductID, res.Level)
	}
	if res.Level == MatchAmbiguous {
		t.Fatal("pack sizes should separate the two, not tie them")
	}
}

func TestBarcodeMatchIsDecisiveWhenTheUserSaysTheColumnIsABarcode(t *testing.T) {
	idx := NewIndex([]MasterProduct{
		{ID: 7, NameAR: "لا يشبه الاسم المطلوب إطلاقاً", Barcode: "6221234567890"},
	})
	opts := DefaultMatchOptions().WithIdentifiers(
		MappedColumns{Barcode: true},
		IdentifierChoices{ByBarcode: true},
	)

	res := idx.Match(&Row{Name: "بانادول إكسترا", Barcode: "6221234567890"}, opts)
	if res.ProductID != 7 || res.Level != MatchBarcode {
		t.Fatalf("barcode match gave %d/%s, want 7/barcode", res.ProductID, res.Level)
	}
}

// The same row with the tier left alone. A GTIN identifies a package and is the
// strongest evidence there is — but only once somebody has said the column
// holds one. Until then an eight-digit internal item number must not be able to
// link a row to a medicine that shares nothing with it but a figure.
func TestBarcodeIsIgnoredUntilTheUserOptsIn(t *testing.T) {
	idx := NewIndex([]MasterProduct{
		{ID: 7, NameAR: "لا يشبه الاسم المطلوب إطلاقاً", Barcode: "6221234567890"},
	})

	res := idx.Match(&Row{Name: "بانادول إكسترا", Barcode: "6221234567890"}, DefaultMatchOptions())
	if res.Level == MatchBarcode {
		t.Fatalf("barcode settled the match with the tier off; product %d", res.ProductID)
	}
}

// A choice the mapping does not support is dropped rather than obeyed: a stored
// setting can outlive the column it was made about.
func TestIdentifierChoicesRequireAMappedColumn(t *testing.T) {
	opts := DefaultMatchOptions().WithIdentifiers(
		MappedColumns{}, // nothing mapped
		IdentifierChoices{ByCode: true, ByBarcode: true, CodeIsCatalogCode: true},
	)
	if opts.TrustBarcode || opts.TrustSupplierCode || opts.CodeIsAuthoritative {
		t.Errorf("identifier tiers enabled with no mapped column: barcode=%v code=%v authoritative=%v",
			opts.TrustBarcode, opts.TrustSupplierCode, opts.CodeIsAuthoritative)
	}

	// And authority cannot outlive the tier it qualifies.
	opts = DefaultMatchOptions().WithIdentifiers(
		MappedColumns{Code: true},
		IdentifierChoices{ByCode: false, CodeIsCatalogCode: true},
	)
	if opts.CodeIsAuthoritative {
		t.Error("code treated as authoritative while code matching is switched off")
	}
}

func TestDefaultOptionsEnableNoIdentifierTier(t *testing.T) {
	opts := DefaultMatchOptions()
	if opts.TrustBarcode || opts.TrustSupplierCode || opts.CodeIsAuthoritative {
		t.Errorf("a tier is on by default: barcode=%v code=%v authoritative=%v",
			opts.TrustBarcode, opts.TrustSupplierCode, opts.CodeIsAuthoritative)
	}
}

func TestUnknownProductIsRefusedRatherThanGuessed(t *testing.T) {
	idx := NewIndex([]MasterProduct{
		{ID: 1, NameAR: "بانادول إكسترا 24 قرص"},
	})
	res := idx.Match(&Row{Name: "زيت زيتون بكر ممتاز 500 مل"}, DefaultMatchOptions())
	if res.Matched() {
		t.Fatalf("matched %d for an unrelated product; the catalogue has nothing like it", res.ProductID)
	}
}

func TestGTINCheckDigit(t *testing.T) {
	cases := map[string]bool{
		"6221234567890": false, // an invented number
		"5449000000996": true,  // a real GTIN-13
		"4006381333931": true,
		"12345":         false,
	}
	for code, want := range cases {
		if got := sheet.ValidGTIN(code); got != want {
			t.Fatalf("ValidGTIN(%s) = %v, want %v", code, got, want)
		}
	}
}

func TestDecimalCoercionHandlesSupplierFormatting(t *testing.T) {
	cases := map[string]string{
		"1,234.50":    "1234.50",
		"1.234,50":    "1234.50",
		"115.00 ج.م":  "115.00",
		"(12.50)":     "-12.50",
		"٢٥٫٥٠":       "25.50",
		"79.90000153": "79.90",
		"20%":         "20",
	}
	for in, want := range cases {
		got, err := sheet.Coerce(in)
		if err != nil {
			t.Fatalf("Coerce(%q): %v", in, err)
		}
		if got.Canonical != want {
			t.Fatalf("Coerce(%q) = %s, want %s", in, got.Canonical, want)
		}
	}
}

func TestExpiryDatesAreReadNotGuessed(t *testing.T) {
	if _, err := sheet.CoerceDate("1785345031-1"); err == nil {
		t.Fatal("a product code must not parse as a date")
	}
	res, err := sheet.CoerceDate("11/2027")
	if err != nil {
		t.Fatalf("month/year date: %v", err)
	}
	if !res.MonthOnly || res.Time.Format("2006-01-02") != "2027-11-30" {
		t.Fatalf("11/2027 read as %s (monthOnly=%v), want the end of November 2027",
			res.Time.Format("2006-01-02"), res.MonthOnly)
	}
}

// The same file must produce the same mapping on every run: the review the
// vendor confirmed is re-derived at processing time and has to agree.
func TestAnalysisIsDeterministic(t *testing.T) {
	fixture := priceList("الكود,الصنف,سعر ج,القائمة")
	first := analyse(t, fixture)
	for i := 0; i < 5; i++ {
		again := analyse(t, fixture)
		for field, col := range first.Mapping.ByField {
			other, ok := again.Mapping.Column(field)
			if !ok || other != col {
				t.Fatalf("run %d bound %s to %d, first run bound it to %d", i, field, other, col)
			}
		}
		if len(again.Mapping.ByField) != len(first.Mapping.ByField) {
			t.Fatalf("run %d bound %d fields, first run bound %d",
				i, len(again.Mapping.ByField), len(first.Mapping.ByField))
		}
	}
}
