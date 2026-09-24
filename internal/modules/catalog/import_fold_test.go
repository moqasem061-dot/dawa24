package catalog

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

func foldTestProduct(name, sku, concentration, form string) *Product {
	return &Product{
		Name:          i18n.Text{"ar": name},
		SKU:           sku,
		Concentration: concentration,
		DosageForm:    form,
	}
}

// The reason folding is strength-aware rather than a key over the name.
//
// A distributor's export lists one product per warehouse and per batch, which
// is what folding is for. It also lists every strength of a brand under the
// same name, which is what folding must not touch: merging those drops a
// medicine the pharmacy can then no longer order.
func TestFoldDuplicatesKeepsStrengthsApart(t *testing.T) {
	products := []*Product{
		foldTestProduct("اوجمنتين", "", "375 مجم", ""),
		foldTestProduct("اوجمنتين", "", "1 جم", ""),
		foldTestProduct("اوجمنتين", "", "375 مجم", ""),
	}

	kept, rows, folded := foldDuplicates(products, []int{2, 3, 4})

	if len(kept) != 2 {
		t.Fatalf("got %d products, want 2 (the two strengths)", len(kept))
	}
	if folded != 1 {
		t.Fatalf("folded %d rows, want 1", folded)
	}
	if len(rows) != 2 || rows[0] != 2 || rows[1] != 3 {
		t.Fatalf("source rows %v, want [2 3]: the row numbers must follow the kept products", rows)
	}
	if kept[0].Concentration != "375 مجم" || kept[1].Concentration != "1 جم" {
		t.Fatalf("strengths were altered: %q then %q",
			kept[0].Concentration, kept[1].Concentration)
	}
}

// The case folding exists for: one product on two lines, each carrying half of
// what the catalogue needs.
func TestFoldDuplicatesFillsGapsFromTheLaterRow(t *testing.T) {
	first := foldTestProduct("بانادول اكسترا", "PAN-1", "", "")
	second := foldTestProduct("بانادول اكسترا", "PAN-1", "", "")
	second.Barcode = "6221234567890"

	kept, _, folded := foldDuplicates([]*Product{first, second}, []int{2, 3})

	if len(kept) != 1 || folded != 1 {
		t.Fatalf("got %d products and %d folded, want 1 and 1", len(kept), folded)
	}
	if kept[0].Barcode != "6221234567890" {
		t.Fatalf("the barcode from the second line was lost: %q", kept[0].Barcode)
	}
}

// A different pharmaceutical form under one name is a different product, even
// when the supplier's own item code says otherwise.
func TestFoldDuplicatesKeepsFormsApart(t *testing.T) {
	products := []*Product{
		foldTestProduct("بروفين", "IBU-1", "", "شراب"),
		foldTestProduct("بروفين", "IBU-1", "", "اقراص"),
	}

	kept, _, folded := foldDuplicates(products, []int{2, 3})
	if len(kept) != 2 || folded != 0 {
		t.Fatalf("got %d products and %d folded, want 2 and 0", len(kept), folded)
	}
}

// A row with nothing identifying is never folded into anything: an empty key
// shared by every unreadable line would merge them all into one.
func TestFoldDuplicatesLeavesUnidentifiableRowsAlone(t *testing.T) {
	products := []*Product{
		{Name: i18n.Text{"ar": ""}},
		{Name: i18n.Text{"ar": ""}},
	}

	kept, _, folded := foldDuplicates(products, []int{2, 3})
	if len(kept) != 2 || folded != 0 {
		t.Fatalf("got %d products and %d folded, want 2 and 0", len(kept), folded)
	}
}

// Folding is the administrator's choice and is off unless they ask for it.
func TestParseSheetFoldsOnlyWhenAsked(t *testing.T) {
	rows := [][]string{
		{"اسم الصنف", "السعر"},
		{"بانادول اكسترا", "25.00"},
		{"بانادول اكسترا", "25.00"},
	}
	data := &SheetData{Rows: rows, Width: 2, Format: "csv"}

	off := ParseSheet(data, LayoutOverrides{}, ImportOptions{})
	if len(off.Products) != 2 {
		t.Fatalf("with the switch off: got %d products, want both rows kept", len(off.Products))
	}
	if off.Stats.DuplicateRows != 0 {
		t.Errorf("with the switch off: reported %d duplicates, want 0", off.Stats.DuplicateRows)
	}

	on := ParseSheet(data, LayoutOverrides{}, ImportOptions{MergeDuplicateRows: true})
	if len(on.Products) != 1 {
		t.Fatalf("with the switch on: got %d products, want 1", len(on.Products))
	}
	if on.Stats.DuplicateRows != 1 {
		t.Errorf("with the switch on: reported %d duplicates, want 1", on.Stats.DuplicateRows)
	}
}
