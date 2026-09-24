package ingest

import (
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// decide applies the import mode and builds the variant to write, or records
// why the row is not being written at all.
func (c *commitRun) decide(sr *RowOutcome) *plannedRow {
	productID := productOf(sr)
	if productID <= 0 {
		c.skipped++
		c.record(sr, OutcomeSkipped, nil, i18n.TDefault("w4_mod.s_379_379"))
		return nil
	}

	variantID, _ := c.variants.resolve(sr.Payload, productID, sr.VariantID)

	// An item is existing in destination:
	// When scoped to a warehouse, only if it currently holds a balance in this warehouse.
	// Otherwise, if the vendor already has this variant in their catalog.
	existsHere := variantID > 0
	if c.settings.WarehouseID > 0 && c.variants.inWarehouse != nil {
		existsHere = variantID > 0 && c.variants.inWarehouse[variantID]
	}

	switch c.settings.Mode {
	case ModeAddOnly:
		if existsHere {
			c.skipped++
			c.record(sr, OutcomeSkipped, nullableVariant(variantID), i18n.TDefault("w4_mod.w4str_209_209"))
			return nil
		}
	case ModeUpdateOnly:
		if !existsHere {
			c.skipped++
			c.record(sr, OutcomeSkipped, nullableVariant(variantID), i18n.TDefault("w4_mod.w4str_208_208"))
			return nil
		}
	}

	return &plannedRow{
		row:        sr,
		variant:    c.buildVariant(sr, productID, variantID),
		existingID: variantID,
	}
}

// buildVariant renders a reviewed row as the vendor's own catalogue entry.
func (c *commitRun) buildVariant(sr *RowOutcome, productID, variantID int64) *VariantDraft {
	v := &VariantDraft{
		ID:             variantID,
		OrganizationID: c.session.OrganizationID,
		ProductID:      productID,
		Name:           i18n.New(sr.EffectiveVariantName(), ""),
		SKU:            sr.SourceCode,
		BranchID:       c.settings.BranchID,
		IsNegotiable:   c.settings.MarkNegotiable,
		Status:         c.statusFor(variantID),
		VariantType:    "standard",
		MinOrderQty:    c.settings.DefaultMinOrderQty,
	}
	if row := sr.Payload; row != nil {
		v.Barcode = row.Barcode
		v.Unit = row.Unit
		v.BatchNumber = row.BatchNumber
		v.ExpiryDate = row.ExpiryDate
		if row.MinOrderQty > 0 {
			v.MinOrderQty = row.MinOrderQty
		}
		if !row.CostPrice.IsZero() {
			cost := row.CostPrice
			v.CostPrice = &cost
		}
		if row.CostDiscountBps > 0 {
			v.CostDiscountBps = row.CostDiscountBps
		}
		if row.Negotiable != nil {
			v.IsNegotiable = *row.Negotiable
		}
		v.Price, v.Discount = listAndDiscount(row)
	}
	return v
}

// statusFor applies "publish immediately".
func (c *commitRun) statusFor(variantID int64) VariantStatus {
	if variantID > 0 {
		return ""
	}
	if c.settings.PublishImmediately {
		return VariantActive
	}
	return VariantInactive
}

// stockFor turns a written row into the warehouse balance it implies, or nil
// where there is nothing to write.
func (c *commitRun) stockFor(sr *RowOutcome, variantID int64) *StockWriteRow {
	if c.svc.inventory == nil || c.settings.WarehouseID <= 0 || variantID <= 0 {
		return nil
	}
	if c.settings.Mode == ModeUpdateOnly && c.variants.inWarehouse != nil && !c.variants.inWarehouse[variantID] {
		return nil
	}
	row := sr.Payload
	if row == nil {
		return nil
	}

	hasQuantity := row.HasQuantity
	quantity := row.Quantity
	if !hasQuantity && c.settings.BlankQuantityIsZero {
		hasQuantity = true
		quantity = 0
	}

	return &StockWriteRow{
		HasQuantity:    hasQuantity,
		OrganizationID: c.session.OrganizationID,
		WarehouseID:    c.settings.WarehouseID,
		ProductID:      productOf(sr),
		VariantID:      variantID,
		Quantity:       quantity,
		MinThreshold:   stockThreshold(row.MinThreshold, c.settings.DefaultMinThreshold),
	}
}
