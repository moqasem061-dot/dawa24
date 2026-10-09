package sheet_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
	"github.com/xuri/excelize/v2"
)

func TestRawPercentCellsKeepStoredValues(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetRow("Sheet1", "A1", &[]any{"Discount", "Price"}); err != nil {
		t.Fatal(err)
	}
	if err := f.SetSheetRow("Sheet1", "A2", &[]any{0.25, 123.45}); err != nil {
		t.Fatal(err)
	}
	style, err := f.NewStyle(&excelize.Style{NumFmt: 9})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStyle("Sheet1", "A2", "A2", style); err != nil {
		t.Fatal(err)
	}
	content, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := sheet.ReadRows(content.Bytes(), "supplier.xlsx", sheet.WithRawCellValues())
	if err != nil {
		t.Fatal(err)
	}
	if rows[1][0] != "0.25" || rows[1][1] != "123.45" {
		t.Fatalf("raw cells = %v", rows[1])
	}
	formatted, err := sheet.ReadRows(content.Bytes(), "supplier.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if formatted[1][0] != "25%" {
		t.Fatalf("default display formatting changed: %v", formatted[1])
	}
}
