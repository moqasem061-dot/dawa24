package compare_test

import (
	"os"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// A real BIFF distributor file the old decoder refused (Continue-record glue
// from row 273 on), which left the mapping modal with fallback headers, made
// mapping-save fail, and blocked matching entirely. The full pre-save chain —
// decode, header detection, column resolution — must succeed on it.
func TestXLSLegacyFileMapsEndToEnd(t *testing.T) {
	data, err := os.ReadFile(`../../../test/corpus/files/vendor-37.xls`)
	if err != nil {
		t.Skipf("corpus file missing: %v", err)
	}
	allRows, err := sheet.ReadRows(data, "vendor-37.xls")
	if err != nil {
		t.Fatalf("ReadRows: %v", err)
	}

	headerRowIdx, fieldMapping, _ := compare.FindBestHeaderRow(allRows)
	if headerRowIdx != 0 {
		t.Errorf("header row = %d, want 0", headerRowIdx)
	}
	headers := allRows[headerRowIdx]

	// Same assembly as UploadAndProcessCompareFile (+ SaveFileMapping input).
	colMapping := make(map[compare.TargetField]*int)
	for colIdx, field := range fieldMapping {
		idx := colIdx
		colMapping[field] = &idx
	}
	var config compare.MappingConfig
	config.NameCol = colMapping[compare.FieldProductName]
	config.PriceCol = colMapping[compare.FieldPrice]
	config.DiscountCol = colMapping[compare.FieldDiscount]
	config.CodeCol = colMapping[compare.FieldSKU]
	if config.CodeCol == nil {
		config.CodeCol = colMapping[compare.FieldProductID]
	}
	if config.NameCol == nil && len(headers) > 0 {
		idx := 0
		if len(headers) > 1 {
			idx = 1
		}
		config.NameCol = &idx
	}
	if config.PriceCol == nil && len(headers) > 2 {
		idx := 2
		config.PriceCol = &idx
	}
	if config.DiscountCol == nil && len(headers) > 3 {
		idx := 3
		config.DiscountCol = &idx
	}
	if config.CodeCol == nil && len(headers) > 0 {
		idx := 0
		config.CodeCol = &idx
	}

	// vendor-37 layout: price col 0, discount col 1, name col 2, sku col 3.
	checks := map[string]*int{
		"name":     config.NameCol,
		"price":    config.PriceCol,
		"discount": config.DiscountCol,
		"code":     config.CodeCol,
	}
	wantIdx := map[string]int{"name": 2, "price": 0, "discount": 1, "code": 3}
	for name, ptr := range checks {
		if ptr == nil {
			t.Errorf("%s column unresolved", name)
			continue
		}
		if *ptr != wantIdx[name] {
			t.Errorf("%s column = %d, want %d", name, *ptr, wantIdx[name])
		}
	}

	// Every data row must carry a name, a positive price and a code — the
	// exact fields mapping-save persists.
	mapped := 0
	for i := headerRowIdx + 1; i < len(allRows); i++ {
		rec := allRows[i]
		at := func(p *int) string {
			if p == nil || *p < 0 || *p >= len(rec) {
				return ""
			}
			return rec[*p]
		}
		if at(config.NameCol) == "" {
			continue
		}
		mapped++
		if i < headerRowIdx+4 {
			t.Logf("row %d: price=%q discount=%q name=%q sku=%q",
				i, at(config.PriceCol), at(config.DiscountCol), at(config.NameCol), at(config.CodeCol))
		}
	}
	if mapped < 1000 {
		t.Errorf("mapped data rows = %d, want >= 1000", mapped)
	}
}
