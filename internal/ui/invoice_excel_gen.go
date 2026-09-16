package ui

import (
	"fmt"
	"io"

	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
)

// GenerateInvoiceExcel builds an authentic Excel spreadsheet for an invoice.
func GenerateInvoiceExcel(data *billing.PrintableInvoiceData, w io.Writer) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	const sheet = "Sheet1"
	sheetName := "فاتورة ضريبية"
	_ = f.SetSheetName(sheet, sheetName)
	_ = f.SetSheetView(sheetName, 0, &excelize.ViewOptions{RightToLeft: boolPtr(true)})

	// Styles
	titleStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 16, Color: "#0F172A"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	subTitleStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Color: "#475569"},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#0284C7"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	metaLabelStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 9, Color: "#334155"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#F1F5F9"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"},
	})
	metaValueStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 9, Color: "#0F172A"},
		Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"},
	})
	boldStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Color: "#0F172A"},
		Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"},
	})
	totalRowStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#0F172A"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"},
	})

	// 1. Header Banner
	_ = f.MergeCell(sheetName, "A1", "J1")
	_ = f.SetCellValue(sheetName, "A1", "منصة دوا 24 - فاتورة استلام وتوريد ضريبية رسمية")
	_ = f.SetCellStyle(sheetName, "A1", "J1", titleStyle)
	_ = f.SetRowHeight(sheetName, 1, 30)

	_ = f.MergeCell(sheetName, "A2", "J2")
	subText := fmt.Sprintf("رقم الفاتورة: %s | تاريخ الإصدار: %s | تاريخ الاستحقاق: %s",
		data.InvoiceNumber, data.IssueDate.Format("2006-01-02"), data.DueDate.Format("2006-01-02"))
	_ = f.SetCellValue(sheetName, "A2", subText)
	_ = f.SetCellStyle(sheetName, "A2", "J2", subTitleStyle)

	// 2. Parties Meta Block
	vendorName := data.Vendor.DisplayName
	if vendorName == "" {
		vendorName = data.Vendor.LegalName
	}
	customerName := data.Customer.DisplayName
	if customerName == "" {
		customerName = data.Customer.LegalName
	}

	_ = f.SetCellValue(sheetName, "A4", "بيانات المورد (البائع)")
	_ = f.SetCellStyle(sheetName, "A4", "A4", metaLabelStyle)
	_ = f.SetCellValue(sheetName, "B4", vendorName)
	_ = f.SetCellStyle(sheetName, "B4", "B4", boldStyle)

	_ = f.SetCellValue(sheetName, "A5", "الرقم الضريبي للمورد")
	_ = f.SetCellStyle(sheetName, "A5", "A5", metaLabelStyle)
	_ = f.SetCellValue(sheetName, "B5", data.Vendor.TaxNumber)
	_ = f.SetCellStyle(sheetName, "B5", "B5", metaValueStyle)

	_ = f.SetCellValue(sheetName, "E4", "بيانات العميل (الصيدلية/المشتري)")
	_ = f.SetCellStyle(sheetName, "E4", "E4", metaLabelStyle)
	_ = f.SetCellValue(sheetName, "F4", customerName)
	_ = f.SetCellStyle(sheetName, "F4", "F4", boldStyle)

	_ = f.SetCellValue(sheetName, "E5", "الرقم الضريبي للعميل")
	_ = f.SetCellStyle(sheetName, "E5", "E5", metaLabelStyle)
	_ = f.SetCellValue(sheetName, "F5", data.Customer.TaxNumber)
	_ = f.SetCellStyle(sheetName, "F5", "F5", metaValueStyle)

	// 3. Items Table Headers
	headers := []string{
		"#", "الكود", "اسم الصنف", "رقم التشغيلة", "تاريخ الصلاحية",
		"الكمية", "سعر الجمهور للقطعة (ج.م)", "الخصم", "صافي سعر الوحدة (ج.م)", "الإجمالي (ج.م)",
	}
	startRow := 7
	for colIdx, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, startRow)
		_ = f.SetCellValue(sheetName, cell, h)
		_ = f.SetCellStyle(sheetName, cell, cell, headerStyle)
	}
	_ = f.SetRowHeight(sheetName, startRow, 24)

	// 4. Rows
	curRow := startRow + 1
	for idx, line := range data.Lines {
		discStr := formatDiscountPercent(line.DiscountPercent)

		_ = f.SetCellValue(sheetName, fmt.Sprintf("A%d", curRow), idx+1)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("B%d", curRow), line.SKU)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("C%d", curRow), line.ItemName)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("D%d", curRow), line.BatchNumber)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("E%d", curRow), line.ExpiryDate)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("F%d", curRow), line.Quantity)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("G%d", curRow), line.UnitPrice.String())
		_ = f.SetCellValue(sheetName, fmt.Sprintf("H%d", curRow), discStr)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("I%d", curRow), line.NetUnitPrice.String())
		_ = f.SetCellValue(sheetName, fmt.Sprintf("J%d", curRow), line.TotalPrice.String())
		curRow++
	}

	// 5. Totals Block
	curRow++
	_ = f.SetCellValue(sheetName, fmt.Sprintf("H%d", curRow), "إجمالي الأصناف قبل الخصم:")
	_ = f.SetCellValue(sheetName, fmt.Sprintf("J%d", curRow), data.Subtotal.String()+" ج.م")
	_ = f.SetCellStyle(sheetName, fmt.Sprintf("H%d", curRow), fmt.Sprintf("H%d", curRow), metaLabelStyle)

	curRow++
	_ = f.SetCellValue(sheetName, fmt.Sprintf("H%d", curRow), "إجمالي الخصم التجاري:")
	_ = f.SetCellValue(sheetName, fmt.Sprintf("J%d", curRow), "-"+data.TotalDiscount.String()+" ج.م")
	_ = f.SetCellStyle(sheetName, fmt.Sprintf("H%d", curRow), fmt.Sprintf("H%d", curRow), metaLabelStyle)

	curRow++
	_ = f.SetCellValue(sheetName, fmt.Sprintf("H%d", curRow), "إجمالي ضريبة القيمة المضافة:")
	_ = f.SetCellValue(sheetName, fmt.Sprintf("J%d", curRow), data.TotalTax.String()+" ج.م")
	_ = f.SetCellStyle(sheetName, fmt.Sprintf("H%d", curRow), fmt.Sprintf("H%d", curRow), metaLabelStyle)

	curRow++
	_ = f.SetCellValue(sheetName, fmt.Sprintf("H%d", curRow), "الصافي الإجمالي المستحق:")
	_ = f.SetCellValue(sheetName, fmt.Sprintf("J%d", curRow), data.TotalAmount.String()+" ج.م")
	_ = f.SetCellStyle(sheetName, fmt.Sprintf("H%d", curRow), fmt.Sprintf("J%d", curRow), totalRowStyle)

	// Set column widths
	_ = f.SetColWidth(sheetName, "A", "A", 6)
	_ = f.SetColWidth(sheetName, "B", "B", 16)
	_ = f.SetColWidth(sheetName, "C", "C", 32)
	_ = f.SetColWidth(sheetName, "D", "D", 16)
	_ = f.SetColWidth(sheetName, "E", "E", 16)
	_ = f.SetColWidth(sheetName, "F", "F", 10)
	_ = f.SetColWidth(sheetName, "G", "G", 18)
	_ = f.SetColWidth(sheetName, "H", "H", 14)
	_ = f.SetColWidth(sheetName, "I", "I", 18)
	_ = f.SetColWidth(sheetName, "J", "J", 20)

	return f.Write(w)
}
