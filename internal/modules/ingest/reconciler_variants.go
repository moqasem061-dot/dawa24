package ingest

import (
	"context"
	"fmt"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func (s *Service) createVariantAndStock(
	ctx context.Context,
	orgID, productID, warehouseID int64,
	rawName, rawSKU, rawBarcode, rawUnit string,
	price, costPrice, discount money.Amount,
	costDiscount float64,
	qty, minThreshold, minOrderQty int,
	catAdapter CatalogAdapter,
	invAdapter InventoryAdapter,
) (*catalog.ProductVariant, error) {
	if catAdapter == nil {
		return nil, fmt.Errorf("catalog adapter unavailable")
	}

	if rawSKU == "" {
		rawSKU = fmt.Sprintf("SKU-%d-%d", productID, time.Now().UnixNano()%100000)
	}

	v := &catalog.ProductVariant{
		OrganizationID:         orgID,
		ProductID:              productID,
		Name:                   i18n.Text{"ar": rawName, "en": rawName},
		SKU:                    rawSKU,
		Barcode:                rawBarcode,
		Price:                  price,
		Discount:               discount,
		CostDiscountPercentage: costDiscount,
		Unit:                   rawUnit,
		MinOrderQty:            minOrderQty,
		Status:                 catalog.StatusActive,
	}
	if !costPrice.IsZero() {
		v.CostPrice = &costPrice
	}

	created, err := catAdapter.CreateVariant(ctx, v)
	if err != nil {
		return nil, fmt.Errorf("create variant: %w", err)
	}

	if warehouseID > 0 && invAdapter != nil {
		stock := &inventory.Stock{
			OrganizationID:   orgID,
			WarehouseID:      warehouseID,
			ProductID:        productID,
			ProductVariantID: created.ID,
			Quantity:         qty,
			MinThreshold:     minThreshold,
		}
		_ = invAdapter.UpsertStock(ctx, stock)
	}

	return created, nil
}

func (s *Service) updateVariantAndStock(
	ctx context.Context,
	v *catalog.ProductVariant,
	warehouseID int64,
	price, costPrice, discount money.Amount,
	costDiscount float64,
	rawUnit, rawBarcode, rawSKU string,
	qty, minThreshold int,
	catAdapter CatalogAdapter,
	invAdapter InventoryAdapter,
) error {
	if catAdapter == nil {
		return fmt.Errorf("catalog adapter unavailable")
	}

	if price.IsPositive() {
		v.Price = price
	}
	if costPrice.IsPositive() {
		v.CostPrice = &costPrice
	}
	if costDiscount > 0 {
		v.CostDiscountPercentage = costDiscount
	}
	if !discount.IsNegative() {
		v.Discount = discount
	}
	if rawUnit != "" {
		v.Unit = rawUnit
	}
	if rawBarcode != "" {
		v.Barcode = rawBarcode
	}
	if rawSKU != "" {
		v.SKU = rawSKU
	}

	if _, err := catAdapter.UpdateVariant(ctx, v.ID, v); err != nil {
		return fmt.Errorf("update variant: %w", err)
	}

	if warehouseID > 0 && invAdapter != nil {
		stock := &inventory.Stock{
			OrganizationID:   v.OrganizationID,
			WarehouseID:      warehouseID,
			ProductID:        v.ProductID,
			ProductVariantID: v.ID,
			Quantity:         qty,
			MinThreshold:     minThreshold,
		}
		_ = invAdapter.UpsertStock(ctx, stock)
	}

	return nil
}
