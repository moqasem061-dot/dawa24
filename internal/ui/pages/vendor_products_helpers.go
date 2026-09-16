package pages

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func vendorPaginationQuery(filter catalog.VendorVariantQuery) url.Values {
	q := url.Values{}
	if filter.Query != "" {
		q.Set("q", filter.Query)
	}
	if filter.Status != "" {
		q.Set("status", filter.Status)
	}
	if filter.Stock != "" {
		q.Set("stock", string(filter.Stock))
	}
	if filter.Expiring {
		q.Set("expiring", "1")
	}
	if filter.Sort != "" {
		q.Set("sort", filter.Sort)
	}
	if filter.PerPage > 0 {
		q.Set("limit", fmt.Sprintf("%d", filter.PerPage))
	}
	return q
}

func vendorFilterURL(filter catalog.VendorVariantQuery, stock catalog.StockFilter, expiring bool) string {
	q := url.Values{}
	if filter.Query != "" {
		q.Set("q", filter.Query)
	}
	if filter.Status != "" {
		q.Set("status", filter.Status)
	}
	if stock != "" {
		q.Set("stock", string(stock))
	}
	if expiring {
		q.Set("expiring", "1")
	}
	if filter.Sort != "" {
		q.Set("sort", filter.Sort)
	}
	if filter.PerPage > 0 {
		q.Set("limit", fmt.Sprintf("%d", filter.PerPage))
	}
	encoded := q.Encode()
	if encoded == "" {
		return "/vendor/products"
	}
	return "/vendor/products?" + encoded
}

func vendorStatusFilterURL(filter catalog.VendorVariantQuery, status string) string {
	q := url.Values{}
	if filter.Query != "" {
		q.Set("q", filter.Query)
	}
	if status != "" {
		q.Set("status", status)
	}
	if filter.Stock != "" {
		q.Set("stock", string(filter.Stock))
	}
	if filter.Expiring {
		q.Set("expiring", "1")
	}
	if filter.Sort != "" {
		q.Set("sort", filter.Sort)
	}
	if filter.PerPage > 0 {
		q.Set("limit", fmt.Sprintf("%d", filter.PerPage))
	}
	encoded := q.Encode()
	if encoded == "" {
		return "/vendor/products"
	}
	return "/vendor/products?" + encoded
}

func formatExpiryDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

func formatCostPrice(v *catalog.ProductVariant) string {
	if v == nil || v.CostPrice == nil || v.CostPrice.IsZero() {
		return ""
	}
	return v.CostPrice.String()
}

func branchIDStr(id *int64) string {
	if id != nil && *id > 0 {
		return fmt.Sprintf("%d", *id)
	}
	return ""
}

// variantEditJSON is everything the edit dialog needs, as one attribute.
func variantEditJSON(v *catalog.ProductVariant) string {
	if v == nil {
		return "{}"
	}
	payload := struct {
		ID           int64   `json:"id"`
		NameAr       string  `json:"name_ar"`
		NameEn       string  `json:"name_en"`
		SKU          string  `json:"sku"`
		Barcode      string  `json:"barcode"`
		Unit         string  `json:"unit"`
		Price        string  `json:"price"`
		Discount     string  `json:"discount"`
		CostPrice    string  `json:"cost_price"`
		CostDiscount string  `json:"cost_discount_percentage"`
		Stock        int     `json:"stock_qty"`
		MinOrderQty  int     `json:"min_order_qty"`
		QuotaLimit   string  `json:"quota_limit"`
		Batch        string  `json:"batch_number"`
		Expiry       string  `json:"expiry_date"`
		BranchID     string  `json:"branch_id"`
		Status       string  `json:"status"`
		IsNegotiable bool    `json:"is_negotiable"`
	}{
		ID:           v.ID,
		NameAr:       v.Name.Get(i18n.AR),
		NameEn:       v.Name.Get(i18n.EN),
		SKU:          v.SKU,
		Barcode:      v.Barcode,
		Unit:         v.Unit,
		Price:        v.Price.String(),
		Discount:     v.Discount.String(),
		CostPrice:    formatCostPrice(v),
		CostDiscount: fmt.Sprintf("%.2f", v.CostDiscountPercentage),
		Stock:        v.StockQty,
		MinOrderQty:  v.MinOrderQty,
		QuotaLimit:   formatQuotaLimit(v),
		Batch:        v.BatchNumber,
		Expiry:       formatExpiryDate(v.ExpiryDate),
		BranchID:     branchIDStr(v.BranchID),
		Status:       string(v.Status),
		IsNegotiable: v.IsNegotiable,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// formatQuotaLimit renders the per-branch cap for the edit dialog.
func formatQuotaLimit(v *catalog.ProductVariant) string {
	if v == nil || !v.HasQuota() {
		return ""
	}
	return fmt.Sprintf("%d", *v.QuotaLimit)
}