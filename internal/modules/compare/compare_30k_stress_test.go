package compare_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

func Test30KRowsSpreadsheetParsing(t *testing.T) {
	t.Log("Generating 30,000 rows in memory XLSX...")
	startGen := time.Now()
	f := excelize.NewFile()
	sheetName := "Sheet1"
	_ = f.SetCellValue(sheetName, "A1", "كود الصنف")
	_ = f.SetCellValue(sheetName, "B1", "اسم المنتج / الدواء")
	_ = f.SetCellValue(sheetName, "C1", "سعر الجمهور")
	_ = f.SetCellValue(sheetName, "D1", "نسبة الخصم")

	const totalRows = 30000
	for i := 1; i <= totalRows; i++ {
		rowIdx := i + 1
		cellA, _ := excelize.CoordinatesToCellName(1, rowIdx)
		cellB, _ := excelize.CoordinatesToCellName(2, rowIdx)
		cellC, _ := excelize.CoordinatesToCellName(3, rowIdx)
		cellD, _ := excelize.CoordinatesToCellName(4, rowIdx)

		_ = f.SetCellValue(sheetName, cellA, fmt.Sprintf("SKU-%06d", i))
		_ = f.SetCellValue(sheetName, cellB, fmt.Sprintf("بنادول اكسترا 24 قرص تشغيلة رقم %d", i))
		_ = f.SetCellValue(sheetName, cellC, fmt.Sprintf("%.2f", 50.0+float64(i%100)))
		_ = f.SetCellValue(sheetName, cellD, fmt.Sprintf("%.1f%%", 20.0+float64(i%15)))
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write xlsx failed: %v", err)
	}
	fileBytes := buf.Bytes()
	t.Logf("Generated %d rows XLSX (Size: %d KB) in %v", totalRows, len(fileBytes)/1024, time.Since(startGen))

	startParse := time.Now()
	rawRows, err := sheet.ReadRows(fileBytes, "stress.xlsx")
	if err != nil {
		t.Fatalf("ReadRows failed: %v", err)
	}
	if len(rawRows) < totalRows {
		t.Fatalf("expected at least %d rows, got %d", totalRows, len(rawRows))
	}
	t.Logf("Successfully parsed %d rows in %v", len(rawRows), time.Since(startParse))
}
