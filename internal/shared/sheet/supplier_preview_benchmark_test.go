package sheet_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
	"github.com/xuri/excelize/v2"
)

func BenchmarkSupplierMappingPreview(b *testing.B) {
	f := excelize.NewFile()
	defer f.Close()
	for row := 1; row <= 10000; row++ {
		cell, err := excelize.CoordinatesToCellName(1, row)
		if err != nil {
			b.Fatal(err)
		}
		if err := f.SetSheetRow("Sheet1", cell, &[]any{"Panadol Extra 24 tablets", row, 100, 12.34}); err != nil {
			b.Fatal(err)
		}
	}
	content, err := f.WriteToBuffer()
	if err != nil {
		b.Fatal(err)
	}
	b.Run("whole_sheet", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := sheet.ReadRows(content.Bytes(), "supplier.xlsx"); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("bounded_raw_sample", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			book, err := sheet.Open(content.Bytes(), "supplier.xlsx", sheet.WithRawCellValues())
			if err != nil {
				b.Fatal(err)
			}
			_, readErr := book.Peek(200)
			closeErr := book.Close()
			if readErr != nil {
				b.Fatal(readErr)
			}
			if closeErr != nil {
				b.Fatal(closeErr)
			}
		}
	})
}
