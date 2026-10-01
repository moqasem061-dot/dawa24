package pages

import (
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
)

// IdentityDiff represents a change between canonical master data and imported row data.
type IdentityDiff struct {
	Field string
	Old   string
	New   string
}

// RowIdentityDiffs extracts old vs new canonical identity fields for an update row.
func RowIdentityDiffs(row *catalog.StagingRow) []IdentityDiff {
	if row == nil || row.Action != catalog.ActionUpdate || row.MatchedProductID == nil || row.Product == nil {
		return nil
	}
	var diffs []IdentityDiff
	newName := strings.TrimSpace(row.DisplayName())
	oldName := strings.TrimSpace(row.MatchedProductName)
	if oldName != "" && newName != "" && oldName != newName {
		diffs = append(diffs, IdentityDiff{Field: "الاسم", Old: oldName, New: newName})
	}

	newSKU := strings.TrimSpace(row.Product.SKU)
	oldSKU := strings.TrimSpace(row.MatchedProductSKU)
	if oldSKU != "" && newSKU != "" && oldSKU != newSKU {
		diffs = append(diffs, IdentityDiff{Field: "كود الصنف", Old: oldSKU, New: newSKU})
	}

	newBarcode := strings.TrimSpace(row.Product.Barcode)
	oldBarcode := strings.TrimSpace(row.MatchedProductBarcode)
	if oldBarcode != "" && newBarcode != "" && oldBarcode != newBarcode {
		diffs = append(diffs, IdentityDiff{Field: "الباركود", Old: oldBarcode, New: newBarcode})
	}

	newForm := strings.TrimSpace(row.Product.DosageForm)
	oldForm := strings.TrimSpace(row.MatchedProductDosageForm)
	if oldForm != "" && newForm != "" && oldForm != newForm {
		diffs = append(diffs, IdentityDiff{Field: "الشكل الصيدلي", Old: oldForm, New: newForm})
	}

	newStrength := strings.TrimSpace(row.Product.Concentration)
	oldStrength := strings.TrimSpace(row.MatchedProductStrength)
	if oldStrength != "" && newStrength != "" && oldStrength != newStrength {
		diffs = append(diffs, IdentityDiff{Field: "التركيز", Old: oldStrength, New: newStrength})
	}

	newSci := strings.TrimSpace(row.Product.ScientificName)
	oldSci := strings.TrimSpace(row.MatchedProductScientific)
	if oldSci != "" && newSci != "" && oldSci != newSci {
		diffs = append(diffs, IdentityDiff{Field: "المادة الفعالة", Old: oldSci, New: newSci})
	}

	newMfg := strings.TrimSpace(row.Product.ManufacturingCompanies)
	oldMfg := strings.TrimSpace(row.MatchedProductManufacturer)
	if oldMfg != "" && newMfg != "" && oldMfg != newMfg {
		diffs = append(diffs, IdentityDiff{Field: "الشركة المصنعة", Old: oldMfg, New: newMfg})
	}

	return diffs
}
