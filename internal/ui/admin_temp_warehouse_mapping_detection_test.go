package ui

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
)

func TestDetectTempWarehouseColsFollowsHeadersInsteadOfPosition(t *testing.T) {
	code, name, price, discount := detectTempWarehouseCols([]string{"الخصم", "سعر الجمهور", "كود الصنف", "اسم الصنف"}, "", "", "", "")
	if code != 2 || name != 3 || price != 1 || discount != 0 {
		t.Fatalf("detected columns = %d, %d, %d, %d", code, name, price, discount)
	}
}

func TestValidTempWarehouseMappingRequiresDistinctRequiredColumns(t *testing.T) {
	name, price, discount := 2, 1, 0
	if !validTempWarehouseMapping(compare.MappingConfig{NameCol: &name, PriceCol: &price, DiscountCol: &discount}) {
		t.Fatal("valid mapping was rejected")
	}
	if validTempWarehouseMapping(compare.MappingConfig{NameCol: &name, PriceCol: &name}) {
		t.Fatal("mapping with one source column assigned twice was accepted")
	}
}
