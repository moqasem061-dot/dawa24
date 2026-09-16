package ui

import (
	"testing"
)

func TestDetectSavingProductColumns(t *testing.T) {
	t.Run("Standard Arabic Header with SKU and Name", func(t *testing.T) {
		headers := []string{"معرف الصنف (ID)", "اسم صنف الصيدلية", "كود SKU", "الكمية", "سعر الشراء المسجل (ج.م)"}
		sampleRows := [][]string{
			{"8952", "بانادول إكسترا 24 قرص", "PAN-24", "10", "35.50"},
		}

		nameCol, skuCol, qtyCol, priceCol, productIDCol := detectSavingProductColumns(headers, sampleRows, "", "", "", "", "")
		if nameCol != 1 {
			t.Errorf("got nameCol %d, want 1", nameCol)
		}
		if skuCol != 2 {
			t.Errorf("got skuCol %d, want 2", skuCol)
		}
		if qtyCol != 3 {
			t.Errorf("got qtyCol %d, want 3", qtyCol)
		}
		if priceCol != 4 {
			t.Errorf("got priceCol %d, want 4", priceCol)
		}
		if productIDCol != 0 {
			t.Errorf("got productIDCol %d, want 0", productIDCol)
		}
	})

	t.Run("Swapped values inside columns (Heuristic Correction)", func(t *testing.T) {
		// In the user's corrupted spreadsheet, header was 'اسم صنف الصيدلية' but values were '20203380' (SKU)
		// and header was 'كود SKU' but values were 'ليدى سبيد استيك 65 جرام' (Name)
		headers := []string{"معرف الصنف (ID)", "اسم صنف الصيدلية", "كود SKU", "الكمية", "سعر الشراء"}
		sampleRows := [][]string{
			{"8952", "20203380", "ليدى سبيد استيك 65 جرام", "0", "0.00"},
			{"8951", "60202971", "فيم فريش غسول انتميت سكير 250 ملى", "0", "0.00"},
			{"8950", "10105879", "يو ريتشى بدى ميست وايت مسك 250 مل", "0", "0.00"},
		}

		nameCol, skuCol, _, _, _ := detectSavingProductColumns(headers, sampleRows, "", "", "", "", "")
		// Should heuristically detect that Col 2 has the Arabic drug names and Col 1 has the digits!
		if nameCol != 2 {
			t.Errorf("heuristic did not fix swapped nameCol: got %d, want 2", nameCol)
		}
		if skuCol != 1 {
			t.Errorf("heuristic did not fix swapped skuCol: got %d, want 1", skuCol)
		}
	})

	t.Run("Custom Overrides Respected", func(t *testing.T) {
		headers := []string{"ColA", "ColB", "ColC", "ColD"}
		sampleRows := [][]string{
			{"1", "2", "3", "4"},
		}

		nameCol, skuCol, qtyCol, priceCol, _ := detectSavingProductColumns(headers, sampleRows, "0", "1", "2", "3", "")
		if nameCol != 0 || skuCol != 1 || qtyCol != 2 || priceCol != 3 {
			t.Errorf("custom overrides failed: got name=%d, sku=%d, qty=%d, price=%d", nameCol, skuCol, qtyCol, priceCol)
		}
	})
}
