package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The administrator's view of every supplier's stock.
//
// /admin/product-child answered "which suppliers sell this" and stopped there.
// The client's complaint was that it stopped there: an operator needs to know
// which BRANCH an offer sits on, which warehouses hold it, and how much is in
// each — "مش معلومات بسيطة كده".
//
// It also built its display names by loading five hundred organisations and a
// thousand products into maps on every request, which is both unbounded and
// wrong past those limits: a supplier at position 501 rendered with a blank
// name. One join answers all of it.

// AdminVariantFilter narrows the cross-supplier stock listing.
type AdminVariantFilter struct {
	Query          string
	Status         string
	ProductID      int64
	OrganizationID int64
	BranchID       int64
	WarehouseID    int64
	// Stock is "", "in", "out" or "low" — the same vocabulary the vendor's own
	// catalogue filter uses, so the two screens mean the same thing by it.
	Stock string
	// ExpiringSoon limits to variants expiring inside ninety days.
	ExpiringSoon bool

	Limit  int
	Offset int
}

// Normalize clamps the window and drops values the query cannot use.
func (f *AdminVariantFilter) Normalize() {
	f.Query = strings.TrimSpace(f.Query)
	f.Status = strings.TrimSpace(f.Status)
	switch f.Stock {
	case "in", "out", "low":
	default:
		f.Stock = ""
	}
	if f.Limit <= 0 {
		f.Limit = 25
	}
	if f.Limit > 200 {
		f.Limit = 200
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
}

// AdminWarehouseStock is one warehouse's holding of one variant.
type AdminWarehouseStock struct {
	WarehouseID   int64  `json:"warehouse_id"`
	WarehouseName string `json:"warehouse_name"`
	BranchName    string `json:"branch_name"`
	Quantity      int    `json:"quantity"`
}

// AdminVariantRow is one line of the cross-supplier stock listing.
type AdminVariantRow struct {
	VariantID   int64     `json:"variant_id"`
	ProductID   int64     `json:"product_id"`
	VariantName i18n.Text `json:"variant_name"`
	ProductName i18n.Text `json:"product_name"`
	Image       string    `json:"image"`
	// ImageIsParent says the picture came from the master product because the
	// offer has none of its own, so the screen can mark it as such rather than
	// implying the supplier photographed it.
	ImageIsParent bool         `json:"image_is_parent"`
	SKU           string       `json:"sku"`
	Barcode       string       `json:"barcode"`
	BatchNumber   string       `json:"batch_number"`
	Status        string       `json:"status"`
	Price         money.Amount `json:"price"`

	OrganizationID   int64     `json:"organization_id"`
	OrganizationName i18n.Text `json:"organization_name"`
	OrganizationType string    `json:"organization_type"`

	BranchID   *int64    `json:"branch_id,omitempty"`
	BranchName i18n.Text `json:"branch_name"`

	TotalQuantity int                   `json:"total_quantity"`
	MinThreshold  int                   `json:"min_threshold"`
	Warehouses    []AdminWarehouseStock `json:"warehouses"`

	ExpiryDate  *string `json:"expiry_date,omitempty"`
	MinOrderQty int     `json:"min_order_qty"`
	QuotaLimit  int     `json:"quota_limit"`
}

// LowStock reports whether the holding is at or under the supplier's own
// threshold, falling back to five where none is set — the same rule the
// vendor's catalogue uses.
func (r *AdminVariantRow) LowStock() bool {
	if r == nil || r.TotalQuantity <= 0 {
		return false
	}
	threshold := r.MinThreshold
	if threshold < 5 {
		threshold = 5
	}
	return r.TotalQuantity <= threshold
}

// DisplayOrgName returns the localized organization name, falling back across locales or to "منشأة #ID".
func (r *AdminVariantRow) DisplayOrgName(lang string) string {
	if r == nil {
		return ""
	}
	if name := r.OrganizationName.Get(i18n.Lang(lang)); name != "" {
		return name
	}
	if name := r.OrganizationName.Get(i18n.AR); name != "" {
		return name
	}
	if name := r.OrganizationName.Get(i18n.EN); name != "" {
		return name
	}
	return fmt.Sprintf("منشأة #%d", r.OrganizationID)
}

// AdminVariantBackend is the cross-supplier stock listing's persistence.
//
// Optional, like the platform's other admin backends, so an in-memory
// repository keeps compiling and the screen reports itself unavailable rather
// than the process failing to build.
type AdminVariantBackend interface {
	ListAdminVariantRows(ctx context.Context, f AdminVariantFilter) ([]*AdminVariantRow, int, error)
	AdminVariantFilterOptions(ctx context.Context) (AdminVariantOptions, error)
}

// AdminVariantOptions fills the filter bar's selects.
type AdminVariantOptions struct {
	Organizations []AdminFilterOption `json:"organizations"`
	Branches      []AdminFilterOption `json:"branches"`
	Warehouses    []AdminFilterOption `json:"warehouses"`
}

// AdminFilterOption is one entry of a filter select.
type AdminFilterOption struct {
	ID       int64  `json:"id"`
	Label    string `json:"label"`
	ParentID int64  `json:"parent_id,omitempty"`
}

// ListAdminVariantRows returns one filtered page of the cross-supplier listing.
func (s *Service) ListAdminVariantRows(
	ctx context.Context, f AdminVariantFilter,
) ([]*AdminVariantRow, int, error) {
	backend, ok := s.repo.(AdminVariantBackend)
	if !ok {
		return nil, 0, nil
	}
	return backend.ListAdminVariantRows(ctx, f)
}

// AdminVariantFilterOptions returns the values the filter bar offers.
func (s *Service) AdminVariantFilterOptions(ctx context.Context) (AdminVariantOptions, error) {
	backend, ok := s.repo.(AdminVariantBackend)
	if !ok {
		return AdminVariantOptions{}, nil
	}
	return backend.AdminVariantFilterOptions(ctx)
}
