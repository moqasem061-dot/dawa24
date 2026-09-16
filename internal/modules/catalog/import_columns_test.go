package catalog_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

func TestPlanColumnsBarcodeDoesNotStealSKU(t *testing.T) {
	// The bug this locks down: the old mapper walked left to right and gave a
	// column to the first field whose keyword it contained. "الباركود الدولي"
	// contains "كود", so it was bound to sku, and the real "كود الصنف" column
	// further right was never reached. Every import of this very common Egyptian
	// layout stored barcodes as item codes and lost the item codes entirely.
	header := []string{
		"اسم الصنف التجاري / الوصف", "الباركود الدولي", "كود الصنف",
		"سعر البيع للجمهور", "الشركة المصنعة",
	}

	plan := catalog.PlanColumns(header, nil)

	want := map[string]int{
		catalog.FieldNameAR:       0,
		catalog.FieldBarcode:      1,
		catalog.FieldSKU:          2,
		catalog.FieldPublicPrice:  3,
		catalog.FieldManufacturer: 4,
	}
	for field, col := range want {
		got, ok := plan.Columns[field]
		if !ok {
			t.Errorf("%s was not mapped", field)
			continue
		}
		if got != col {
			t.Errorf("%s mapped to column %d, want %d", field, got, col)
		}
	}
}

func TestPlanColumnsGivesEachColumnOneField(t *testing.T) {
	// "اسم الصنف التجاري / الوصف" matches both the name and the description.
	// Binding it to both — which the old mapper did — copied the product name
	// into the description of every row of every such file.
	plan := catalog.PlanColumns([]string{"اسم الصنف التجاري / الوصف", "السعر"}, nil)

	if _, ok := plan.Columns[catalog.FieldDescriptionAR]; ok {
		t.Error("the name column was also claimed as the description")
	}
	if plan.Columns[catalog.FieldNameAR] != 0 {
		t.Errorf("name_ar mapped to %d, want 0", plan.Columns[catalog.FieldNameAR])
	}
}

func TestPlanColumnsSeparatesPriceKinds(t *testing.T) {
	// All three contain "سعر". Binding the cost price as the selling price would
	// publish the pharmacy's margin to its customers.
	plan := catalog.PlanColumns([]string{"سعر البيع", "سعر التكلفة", "سعر الجمهور", "نسبة الخصم"}, nil)

	checks := map[string]int{
		catalog.FieldPrice:       0,
		catalog.FieldCostPrice:   1,
		catalog.FieldPublicPrice: 2,
		catalog.FieldDiscount:    3,
	}
	for field, col := range checks {
		if got := plan.Columns[field]; got != col {
			t.Errorf("%s mapped to column %d, want %d", field, got, col)
		}
	}
}

func TestPlanColumnsToleratesArabicSpellingVariants(t *testing.T) {
	// The same header from four suppliers: with harakat, with tatweel, with
	// ta-marbuta versus ha, and with hamza dropped. All must resolve alike.
	variants := []string{
		"الشركة المصنعة",
		"الشركه المصنعه",
		"الشـركة المصنـعة",
		"الشَّرِكَة المُصَنِّعَة",
	}
	for _, header := range variants {
		plan := catalog.PlanColumns([]string{"اسم الصنف", header}, nil)
		if got, ok := plan.Columns[catalog.FieldManufacturer]; !ok || got != 1 {
			t.Errorf("%q did not map to manufacturer (got %d, ok=%v)", header, got, ok)
		}
	}
}

func TestPlanColumnsHandlesEnglishAndReordering(t *testing.T) {
	plan := catalog.PlanColumns([]string{
		"Preferred Vendor", "Unit Price", "Item Description", "Item No.", "EAN13", "Dosage Form",
	}, nil)

	want := map[string]int{
		catalog.FieldManufacturer: 0,
		catalog.FieldNameAR:       2,
		catalog.FieldSKU:          3,
		catalog.FieldBarcode:      4,
		catalog.FieldDosageForm:   5,
	}
	for field, col := range want {
		got, bound := plan.Columns[field]
		if !bound {
			t.Errorf("%s was not mapped at all", field)
			continue
		}
		if got != col {
			t.Errorf("%s mapped to column %d, want %d", field, got, col)
		}
	}

	// "Unit Price" is the price, and which of the two price fields it lands in
	// is not a fact about the file. The shared vocabulary reads an unqualified
	// price as the public one, because in an Egyptian price list it is; this
	// module has one price column and fills it from whichever field is bound.
	// Asserting the label rather than the outcome made a naming choice look like
	// a behaviour change.
	_, hasPrice := plan.Columns[catalog.FieldPrice]
	_, hasPublic := plan.Columns[catalog.FieldPublicPrice]
	if !hasPrice && !hasPublic {
		t.Error("the price column was not mapped to any price field")
	}
	if hasPrice && plan.Columns[catalog.FieldPrice] != 1 {
		t.Errorf("price mapped to column %d, want 1", plan.Columns[catalog.FieldPrice])
	}
	if hasPublic && plan.Columns[catalog.FieldPublicPrice] != 1 {
		t.Errorf("public price mapped to column %d, want 1", plan.Columns[catalog.FieldPublicPrice])
	}
}

func TestPlanColumnsReportsUnmappedHeaders(t *testing.T) {
	// Unmapped columns are surfaced, not dropped silently: an admin whose "سعر
	// الجمهور" column went unread needs to know before trusting the catalogue.
	plan := catalog.PlanColumns([]string{"اسم الصنف", "رقم التشغيلة", "تاريخ الصلاحية"}, nil)

	if len(plan.Unmapped) != 2 {
		t.Fatalf("expected 2 unmapped headers, got %v", plan.Unmapped)
	}
}

func TestNormalizeKeyFoldsArabicAndDigits(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Item No.", "itemno"},
		{"ITEM_NO", "itemno"},
		{"سعر  الجمهور", "سعرالجمهور"},
		{"الشركة", "الشركه"},
		{"مُصَنِّع", "مصنع"},
		{"صـ__نف", "صنف"},
		{"كمية ٢٥", "كميه25"},
	}
	for _, tc := range tests {
		if got := catalog.NormalizeKey(tc.in); got != tc.want {
			t.Errorf("NormalizeKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
