package pages

import (
	"fmt"
	"net/url"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
)

type VendorWarehouseStockFilter struct {
	Query        string
	StockStatus  string
	ExpiryStatus string
	Sort         string
	Page         int
	PerPage      int
}

// VendorWarehouseStats calculates inventory analytics for a single warehouse.
type VendorWarehouseStats struct {
	TotalItems        int
	TotalUnits        int
	LowStockCount     int
	OutOfStockCount   int
	ExpiringSoonCount int
	ExpiredCount      int
}

// VendorWarehouseDetailData contains all context needed to render warehouse stock management.
type VendorWarehouseDetailData struct {
	Warehouse       *inventory.Warehouse
	OtherWarehouses []*inventory.Warehouse
	Stocks          []*inventory.DetailedWarehouseStockView
	Stats           VendorWarehouseStats
	Filter          VendorWarehouseStockFilter
	TotalFiltered   int
	TotalPages      int
	FromItem        int
	ToItem          int
	NoticeType      string
	NoticeMsg       string
}

func buildWarehouseInventoryURL(whID int64, f VendorWarehouseStockFilter, page, perPage int) string {
	q := url.Values{}
	if f.Query != "" {
		q.Set("q", f.Query)
	}
	if f.StockStatus != "" && f.StockStatus != "all" {
		q.Set("stock_status", f.StockStatus)
	}
	if f.ExpiryStatus != "" && f.ExpiryStatus != "all" {
		q.Set("expiry_status", f.ExpiryStatus)
	}
	if f.Sort != "" && f.Sort != "updated_desc" {
		q.Set("sort", f.Sort)
	}
	if page > 1 {
		q.Set("page", fmt.Sprint(page))
	}
	if perPage > 0 && perPage != 20 {
		q.Set("limit", fmt.Sprint(perPage))
	}
	qs := q.Encode()
	if qs != "" {
		return fmt.Sprintf("/vendor/warehouses/%d?%s", whID, qs)
	}
	return fmt.Sprintf("/vendor/warehouses/%d", whID)
}

func isExpiringSoon(exp *time.Time) bool {
	if exp == nil {
		return false
	}
	now := time.Now()
	sixMonths := now.AddDate(0, 6, 0)
	return exp.After(now) && exp.Before(sixMonths)
}

func isExpired(exp *time.Time) bool {
	if exp == nil {
		return false
	}
	return exp.Before(time.Now())
}
