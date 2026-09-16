package ui

import (
	"encoding/csv"
	"net/http"

	"github.com/xuri/excelize/v2"
)

// SmartOrderSampleXLSX streams download of a clean Excel template for Smart Order deficiency lists.
func (h *UIHandler) SmartOrderSampleXLSX(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Sheet1"
	sheetName := "نواقص الطلب الذكي"
	_ = f.SetSheetName(sheet, sheetName)
	_ = f.SetSheetView(sheetName, 0, &excelize.ViewOptions{RightToLeft: boolPtr(true)})

	headers := []string{"اسم الصنف / الدواء", "كود الصنف (SKU)", "الباركود الدولي", "الكمية المطلوبة"}

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#0284C7"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	_ = f.SetRowHeight(sheetName, 1, 26)

	for i, hName := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheetName, cell, hName)
		_ = f.SetCellStyle(sheetName, cell, cell, headerStyle)
	}

	samples := [][]any{
		{"كونجستال 20 قرص", "CONG-TAB-20", "6221142003322", 30},
		{"بنادول إكسترا 24 قرص", "PAN-EXT-24", "6221142001234", 50},
		{"أوجمنتين 1 جم 14 قرص", "AUG-1G-14", "6221142005678", 20},
		{"كتفلو 50 مجم أكياس", "CATA-50-S", "6221142009999", 15},
		{"أنتينال 24 كبسولة", "ANTIN-24", "6221142004455", 40},
	}

	for rIdx, row := range samples {
		for cIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(cIdx+1, rIdx+2)
			_ = f.SetCellValue(sheetName, cell, val)
		}
	}

	_ = f.SetColWidth(sheetName, "A", "A", 35)
	_ = f.SetColWidth(sheetName, "B", "B", 20)
	_ = f.SetColWidth(sheetName, "C", "C", 20)
	_ = f.SetColWidth(sheetName, "D", "D", 16)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_smart_order_template.xlsx\"")
	_ = f.Write(w)
}

// SmartOrderSampleCSV streams download of a clean CSV template for Smart Order deficiency lists.
func (h *UIHandler) SmartOrderSampleCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_smart_order_template.csv\"")

	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM

	cw := csv.NewWriter(w)
	defer cw.Flush()

	_ = cw.Write([]string{"اسم الصنف / الدواء", "كود الصنف (SKU)", "الباركود الدولي", "الكمية المطلوبة"})
	_ = cw.Write([]string{"كونجستال 20 قرص", "CONG-TAB-20", "6221142003322", "30"})
	_ = cw.Write([]string{"بنادول إكسترا 24 قرص", "PAN-EXT-24", "6221142001234", "50"})
	_ = cw.Write([]string{"أوجمنتين 1 جم 14 قرص", "AUG-1G-14", "6221142005678", "20"})
	_ = cw.Write([]string{"كتفلو 50 مجم أكياس", "CATA-50-S", "6221142009999", "15"})
	_ = cw.Write([]string{"أنتينال 24 كبسولة", "ANTIN-24", "6221142004455", "40"})
}
