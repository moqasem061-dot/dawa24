package ui

import (
	"context"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// attachSavingChangePreviews inspects existing saving entries for the organization
// and attaches delta previews to any staged items updating an existing record.
func (h *UIHandler) attachSavingChangePreviews(ctx context.Context, orgID int64, items []*StagedSavingItem) {
	if h.catSvc == nil || orgID <= 0 || len(items) == 0 {
		return
	}

	sysCtx := database.AsSystem(ctx)
	existingList, err := h.catSvc.ListSavingProducts(sysCtx, orgID, 5000, 0)
	if err != nil || len(existingList) == 0 {
		return
	}

	bySKU := make(map[string]*catalog.SavingProduct, len(existingList))
	byName := make(map[string]*catalog.SavingProduct, len(existingList))
	for _, p := range existingList {
		if p == nil {
			continue
		}
		if s := strings.ToUpper(strings.TrimSpace(p.SKU)); s != "" {
			bySKU[s] = p
		}
		if n := strings.ToLower(strings.TrimSpace(p.NameProduct)); n != "" {
			byName[n] = p
		}
	}

	for _, it := range items {
		if it == nil {
			continue
		}
		var matchedExisting *catalog.SavingProduct
		if s := strings.ToUpper(strings.TrimSpace(it.SKU)); s != "" {
			matchedExisting = bySKU[s]
		}
		if matchedExisting == nil {
			if n := strings.ToLower(strings.TrimSpace(it.NameProduct)); n != "" {
				matchedExisting = byName[n]
			}
		}

		if matchedExisting != nil {
			oldUnit := detectPackUnit(matchedExisting.NameProduct)
			newUnit := it.PackUnit
			if newUnit == "" {
				newUnit = detectPackUnit(it.NameProduct)
			}
			it.ChangePreview = &pages.SavingChangePreview{
				IsUpdate:       true,
				ExistingID:     matchedExisting.ID,
				OldProductID:   matchedExisting.ProductID,
				OldProductName: matchedExisting.NameProduct,
				NewProductID:   it.ProductID,
				NewProductName: it.MasterProductName,
				OldPackUnit:    oldUnit,
				NewPackUnit:    newUnit,
				OldQuantity:    matchedExisting.Quantity,
				NewQuantity:    it.Quantity,
				OldPrice:       matchedExisting.Price,
				NewPrice:       it.Price,
			}
		}
	}
}

// detectPackUnit extracts a packaging unit from the product name, defaulting to "علبة".
func detectPackUnit(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "قرص") || strings.Contains(lower, "أقراص") || strings.Contains(lower, "tablet"):
		return "قرص"
	case strings.Contains(lower, "كبسول") || strings.Contains(lower, "capsule"):
		return "كبسولة"
	case strings.Contains(lower, "أمبول") || strings.Contains(lower, "امبول") || strings.Contains(lower, "ampoule"):
		return "أمبول"
	case strings.Contains(lower, "شريط") || strings.Contains(lower, "strip"):
		return "شريط"
	case strings.Contains(lower, "شراب") || strings.Contains(lower, "syrup"):
		return "زجاجة شراب"
	case strings.Contains(lower, "نقط") || strings.Contains(lower, "drops"):
		return "قطرة"
	case strings.Contains(lower, "مرهم") || strings.Contains(lower, "ointment"):
		return "أنبوبة مرهم"
	case strings.Contains(lower, "كريم") || strings.Contains(lower, "cream"):
		return "أنبوبة كريم"
	case strings.Contains(lower, "فوار") || strings.Contains(lower, "sachet") || strings.Contains(lower, "كيس"):
		return "كيس فوار"
	default:
		return "علبة"
	}
}
