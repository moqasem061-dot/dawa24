package catalog

import (
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// ProductProviderInfo represents an organization/supplier selling a linked master product with their pricing and stock.
type ProductProviderInfo struct {
	OrgID              int64        `json:"org_id"`
	OrgName            i18n.Text    `json:"org_name"`
	VariantID          int64        `json:"variant_id"`
	VariantName        i18n.Text    `json:"variant_name"`
	SKU                string       `json:"sku"`
	Unit               string       `json:"unit"`
	Price              money.Amount `json:"price"`
	Discount           money.Amount `json:"discount"`
	PriceAfterDiscount money.Amount `json:"price_after_discount"`
	StockQuantity      int          `json:"stock_quantity"`
	Status             string       `json:"status"`
	BranchName         i18n.Text    `json:"branch_name"`
}

// SavingProductStats summarizes saving products metrics for a vendor.
type SavingProductStats struct {
	CountAll      int          `json:"count_all"`
	CountLinked   int          `json:"count_linked"`
	CountUnlinked int          `json:"count_unlinked"`
	FilteredCount int          `json:"filtered_count"`
	TotalQuantity float64      `json:"total_quantity"`
	TotalValue    money.Amount `json:"total_value"`
}

// CatalogMatchSource holds core metadata of a master product for high-speed matching.
type CatalogMatchSource struct {
	ID      int64  `json:"id"`
	SKU     string `json:"sku"`
	Barcode string `json:"barcode"`
	NameAr  string `json:"name_ar"`
	NameEn  string `json:"name_en"`
}
