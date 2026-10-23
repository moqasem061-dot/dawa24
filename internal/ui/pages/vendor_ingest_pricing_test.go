package pages

import (
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestVendorMappingPresentsSellingPriceAsDiscountAlternative(t *testing.T) {
	view := VendorImportView{Analysis: &productmatch.Analysis{Mapping: &productmatch.Mapping{Columns: []*productmatch.Column{
		{Index: 0, Field: productmatch.FieldName}, {Index: 1, Field: productmatch.FieldPublicPrice},
		{Index: 2, Field: productmatch.FieldPrice},
	}}}}
	foundSale := false
	for _, row := range view.FieldMappingRows() {
		if row.Spec.Field == productmatch.FieldPrice && row.Bound() {
			t.Fatal("selling-price column offered twice")
		}
		if row.Spec.Field == productmatch.FieldDiscountPct {
			foundSale = row.SalePriceInput && row.ColumnIndex == 2
		}
	}
	if !foundSale {
		t.Fatal("selling price was not selected as the discount alternative")
	}
}

func TestVendorSalePriceFindingUsesTheSelectedLanguage(t *testing.T) {
	if !strings.Contains(vendorImportFindingMessage("vendor.pricing.invalid", "en"), "sale price") {
		t.Fatal("sale-price error did not translate")
	}
	if vendorImportFindingMessage("legacy finding", "en") != "legacy finding" {
		t.Fatal("legacy finding changed")
	}
}
