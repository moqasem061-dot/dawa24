package catalog_test

import (
	"os"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

func TestUserRealWorldExcelFile(t *testing.T) {
	filePath := "C:\\Users\\mydwa\\Downloads\\قايمه الاصناف اكسيل (1).xlsx"
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Skipf("skipping local user file test: %v", err)
		return
	}

	records, err := catalog.ParseUploadedSpreadsheet(content, "قايمه الاصناف اكسيل (1).xlsx")
	if err != nil {
		t.Fatalf("ParseUploadedSpreadsheet failed: %v", err)
	}

	if len(records) < 9000 {
		t.Fatalf("expected at least 9000 records, got %d", len(records))
	}

	products, stats := catalog.ParseProductRows(records)
	if len(products) < 8500 {
		t.Fatalf("expected at least 8500 valid products, got %d", len(products))
	}

	if stats.RepeatedHeader != 114 {
		t.Errorf("expected 114 repeated headers filtered, got %d", stats.RepeatedHeader)
	}

	t.Logf("Successfully extracted %d products (filtered %d repeated print headers, %d empty rows)",
		len(products), stats.RepeatedHeader, stats.EmptyRows)
}
