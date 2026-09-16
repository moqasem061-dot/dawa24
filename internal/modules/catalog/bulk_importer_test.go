package catalog_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

func TestDetectHeaderRow(t *testing.T) {
	records := [][]string{
		{},
		{"Item No.", "Item Description", "Preferred Vendor"},
		{"10202898", "بوباى صن سكرين كريم 50 جم (+50SPF)", "Parkville Pharmaceticals EGYPT"},
		{"10106853", "بريث داى غسول فم بالشاى الاخضر 300 مل", "Parkville Pharmaceticals EGYPT"},
	}

	headerIdx := catalog.DetectHeaderRow(records)
	if headerIdx != 1 {
		t.Fatalf("expected header index 1, got %d", headerIdx)
	}

	colMap := catalog.MapHeaderColumns(records[headerIdx], nil)
	if colMap["sku"] != 0 {
		t.Errorf("expected sku col 0, got %d", colMap["sku"])
	}
	if colMap["name_ar"] != 1 {
		t.Errorf("expected name_ar col 1, got %d", colMap["name_ar"])
	}
	if colMap["manufacturer"] != 2 {
		t.Errorf("expected manufacturer col 2, got %d", colMap["manufacturer"])
	}
}

func TestFilterRepeatedHeadersAndExtractAttributes(t *testing.T) {
	records := [][]string{
		{},
		{"Item No.", "Item Description", "Preferred Vendor"},
		{"10202898", "بوباى صن سكرين كريم 50 جم (+50SPF) (1+1)", "Parkville Pharmaceticals EGYPT"},
		{},
		{"10106853", "بريث داى غسول فم بالشاى الاخضر 300 مل", "Parkville Pharmaceticals EGYPT"},
		// Repeated print header
		{"Item No.", "Item Description", "Preferred Vendor"},
		{"10107005", "بوباى تانينج جل 300 ملى", "Parkville Pharmaceticals EGYPT"},
		{"101031539", "بوباى زيت تانينج لتسمير البشرة 220 مل", "Parkville Pharmaceticals EGYPT"},
	}

	prods, stats := catalog.ParseProductRows(records)
	if len(prods) != 4 {
		t.Fatalf("expected 4 valid products, got %d", len(prods))
	}
	if stats.RepeatedHeader != 1 {
		t.Errorf("expected 1 repeated header filtered, got %d", stats.RepeatedHeader)
	}
	if stats.EmptyRows != 1 {
		t.Errorf("expected 1 empty row skipped, got %d", stats.EmptyRows)
	}

	// Verify dosage form and concentration extraction
	if prods[0].DosageForm != "كريم" {
		t.Errorf("expected DosageForm 'كريم', got '%s'", prods[0].DosageForm)
	}
	if prods[0].Concentration != "50 جم" {
		t.Errorf("expected Concentration '50 جم', got '%s'", prods[0].Concentration)
	}
	if prods[1].DosageForm != "غسول فم" {
		t.Errorf("expected DosageForm 'غسول فم', got '%s'", prods[1].DosageForm)
	}
	if prods[2].DosageForm != "جل" {
		t.Errorf("expected DosageForm 'جل', got '%s'", prods[2].DosageForm)
	}
	if prods[3].DosageForm != "زيت" {
		t.Errorf("expected DosageForm 'زيت', got '%s'", prods[3].DosageForm)
	}
}
