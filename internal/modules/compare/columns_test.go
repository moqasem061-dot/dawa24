package compare_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
)

func TestDetectColumns_RealWorldHeaders(t *testing.T) {
	testCases := []struct {
		name     string
		headers  []string
		expected map[int]compare.TargetField
	}{
		{
			name:    "Standard Arabic Supplier Sheet",
			headers: []string{"كود الصنف", "اسم الصنف", "السعر", "الخصم", "الكمية المتاحة"},
			expected: map[int]compare.TargetField{
				0: compare.FieldSKU,
				1: compare.FieldProductName,
				2: compare.FieldPrice,
				3: compare.FieldDiscount,
				4: compare.FieldQuantity,
			},
		},
		{
			name:    "Diacritics and Alternative Spellings",
			headers: []string{"إسم الصنف", "السِعْرُ", "نِسْبَةُ الخَصْمِ", "الرَّمْزُ", "البَارْكُود"},
			expected: map[int]compare.TargetField{
				0: compare.FieldProductName,
				1: compare.FieldPrice,
				2: compare.FieldDiscount,
				3: compare.FieldSKU,
				4: compare.FieldBarcode,
			},
		},
		{
			name:    "Standard English Supplier Sheet",
			headers: []string{"Product SKU", "Product Name", "Unit Price", "Discount %", "Stock Level", "Barcode"},
			expected: map[int]compare.TargetField{
				0: compare.FieldSKU,
				1: compare.FieldProductName,
				2: compare.FieldPrice,
				3: compare.FieldDiscount,
				4: compare.FieldQuantity,
				5: compare.FieldBarcode,
			},
		},
		{
			name:    "Mixed Bilingual Sheet",
			headers: []string{"ID", "اسم المنتج (Arabic)", "Price EGP", "الخصم %", "Qty", "Description"},
			expected: map[int]compare.TargetField{
				0: compare.FieldProductID,
				1: compare.FieldProductName,
				2: compare.FieldPrice,
				3: compare.FieldDiscount,
				4: compare.FieldQuantity,
				5: compare.FieldDescription,
			},
		},
		{
			name:    "Technical Database Export",
			headers: []string{"product_id", "product_name", "price", "discount", "quantity", "sku", "unique_id"},
			expected: map[int]compare.TargetField{
				0: compare.FieldProductID,
				1: compare.FieldProductName,
				2: compare.FieldPrice,
				3: compare.FieldDiscount,
				4: compare.FieldQuantity,
				5: compare.FieldSKU,
				6: compare.FieldUniqueID,
			},
		},
		{
			name:    "Shortened Slang Headers",
			headers: []string{"كود", "صنف", "سعر", "اوفر", "متوفر"},
			expected: map[int]compare.TargetField{
				0: compare.FieldProductID,
				1: compare.FieldProductName,
				2: compare.FieldPrice,
				3: compare.FieldDiscount,
				4: compare.FieldQuantity,
			},
		},
		{
			name:    "Headers with BOM and Zero-Width Spaces",
			headers: []string{"\xEF\xBB\xBFاسم الصنف", "\u200Bالسعر\u200C", "الخصم\uFEFF"},
			expected: map[int]compare.TargetField{
				0: compare.FieldProductName,
				1: compare.FieldPrice,
				2: compare.FieldDiscount,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := compare.DetectColumns(tc.headers)
			for idx, expectedField := range tc.expected {
				gotField, ok := result[idx]
				if !ok {
					t.Errorf("expected column %d to be detected as %s, but was not mapped", idx, expectedField)
				} else if gotField != expectedField {
					t.Errorf("column %d (%s): expected %s, got %s", idx, tc.headers[idx], expectedField, gotField)
				}
			}
		})
	}
}

func TestFindBestHeaderRow(t *testing.T) {
	rows := [][]string{
		{"شركة الدواء الحديثة للتوزيع", "", "", ""}, // Title banner row
		{"تاريخ التقرير: 2026-08-19", "", "", ""},   // Metadata row
		{"", "", "", ""}, // Blank row
		{"كود الصنف", "اسم الصنف", "السعر", "الخصم"}, // Real header row (Index 3)
		{"1001", "Panadol Extra 24 Tab", "45.00", "12%"},
		{"1002", "Cataflam 50mg 20 Tab", "38.50", "8%"},
	}

	bestIdx, mapping, confidence := compare.FindBestHeaderRow(rows)
	if bestIdx != 3 {
		t.Errorf("expected best header row index 3, got %d", bestIdx)
	}
	if confidence <= 0.4 {
		t.Errorf("expected high confidence > 0.4, got %f", confidence)
	}
	if mapping[1] != compare.FieldProductName {
		t.Errorf("expected mapping[1] = product_name, got %s", mapping[1])
	}
	if mapping[2] != compare.FieldPrice {
		t.Errorf("expected mapping[2] = price, got %s", mapping[2])
	}
}

func TestValidateMapping(t *testing.T) {
	// 1. Valid mapping (Name + Price)
	validMapping := map[int]compare.TargetField{
		0: compare.FieldProductName,
		1: compare.FieldPrice,
	}
	valid, missing := compare.ValidateMapping(validMapping)
	if !valid || len(missing) > 0 {
		t.Errorf("expected mapping with name+price to be valid, got missing: %v", missing)
	}

	// 2. Valid mapping (Name + Discount)
	validDiscountMapping := map[int]compare.TargetField{
		0: compare.FieldProductName,
		1: compare.FieldDiscount,
	}
	valid, missing = compare.ValidateMapping(validDiscountMapping)
	if !valid || len(missing) > 0 {
		t.Errorf("expected mapping with name+discount to be valid, got missing: %v", missing)
	}

	// 3. Missing Name
	missingNameMapping := map[int]compare.TargetField{
		0: compare.FieldPrice,
		1: compare.FieldDiscount,
	}
	valid, missing = compare.ValidateMapping(missingNameMapping)
	if valid {
		t.Errorf("expected mapping without name to be invalid")
	}

	// 4. Missing Price and Discount
	missingPriceMapping := map[int]compare.TargetField{
		0: compare.FieldProductName,
		1: compare.FieldSKU,
	}
	valid, missing = compare.ValidateMapping(missingPriceMapping)
	if valid {
		t.Errorf("expected mapping without price or discount to be invalid")
	}
}
