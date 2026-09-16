package ui

import (
	"bytes"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/xuri/excelize/v2"
)

func TestInvoiceExportExcelAndWord(t *testing.T) {
	data := &billing.PrintableInvoiceData{
		InvoiceNumber: "INV-2026-0001",
		IssueDate:     time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
		DueDate:       time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC),
		Vendor:        billing.PrintableOrgInfo{DisplayName: "مستودع الأمل للأدوية", TaxNumber: "123-456-789"},
		Customer:      billing.PrintableOrgInfo{DisplayName: "صيدلية النور", TaxNumber: "987-654-321"},
		Lines: []*billing.PrintableInvoiceLine{
			{
				Index:           1,
				ItemName:        "كونجستال 20 قرص",
				SKU:             "CONG-TAB-20",
				Quantity:        10,
				UnitPrice:       money.MustParse("30.00"),
				DiscountPercent: 10.0,
				NetUnitPrice:    money.MustParse("27.00"),
				TotalPrice:      money.MustParse("270.00"),
			},
		},
		Subtotal:      money.MustParse("300.00"),
		TotalDiscount: money.MustParse("30.00"),
		TotalTax:      money.MustParse("0.00"),
		TotalAmount:   money.MustParse("270.00"),
	}

	t.Run("GenerateInvoiceExcel", func(t *testing.T) {
		var buf bytes.Buffer
		err := GenerateInvoiceExcel(data, &buf)
		if err != nil {
			t.Fatalf("GenerateInvoiceExcel failed: %v", err)
		}
		if buf.Len() == 0 {
			t.Fatalf("expected non-empty Excel buffer")
		}

		f, err := excelize.OpenReader(&buf)
		if err != nil {
			t.Fatalf("failed to open generated Excel: %v", err)
		}
		defer f.Close()

		rows, err := f.GetRows("فاتورة ضريبية")
		if err != nil {
			t.Fatalf("failed to get sheet rows: %v", err)
		}
		if len(rows) < 8 {
			t.Fatalf("expected at least 8 rows in invoice sheet, got %d", len(rows))
		}
	})

	t.Run("GenerateInvoiceWord", func(t *testing.T) {
		var buf bytes.Buffer
		err := GenerateInvoiceWord(data, &buf)
		if err != nil {
			t.Fatalf("GenerateInvoiceWord failed: %v", err)
		}
		if buf.Len() == 0 {
			t.Fatalf("expected non-empty Word docx buffer")
		}
		// Verify PK zip header for docx format
		if !bytes.HasPrefix(buf.Bytes(), []byte("PK")) {
			t.Errorf("docx missing zip PK header")
		}
	})
}
