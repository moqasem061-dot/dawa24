package pages

import (
	"net/url"
	"strconv"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

// VendorVariantItem holds a vendor variant with resolved relations.
type VendorVariantItem struct {
	Variant        *catalog.ProductVariant
	DisplayImage   string
	IsParentImage  bool
	OrgName        string
	ParentProdName string
	HasImage       bool
}

// AdminProductChildrenData encapsulates the view model for /admin/product-child.
type AdminProductChildrenData struct {
	// Rows carries the join the screen actually needs: the supplier and branch
	// resolved to names, the total holding, and every warehouse holding it.
	// The page used to build its names by loading five hundred organisations
	// and a thousand products into maps on each request, which was unbounded
	// and blank past those limits.
	Rows    []*catalog.AdminVariantRow
	Options catalog.AdminVariantOptions

	Total   int
	Page    int
	PerPage int

	SearchQuery    string
	StatusFilter   string
	OrganizationID int64
	BranchID       int64
	WarehouseID    int64
	StockFilter    string
	ExpiringSoon   bool
}

// QueryValues renders the active filters so the pager asks the same question
// on page two that the filter bar asked on page one.
func (d AdminProductChildrenData) QueryValues() url.Values {
	v := url.Values{}
	if d.SearchQuery != "" {
		v.Set("q", d.SearchQuery)
	}
	if d.StatusFilter != "" && d.StatusFilter != "all" {
		v.Set("status", d.StatusFilter)
	}
	if d.OrganizationID > 0 {
		v.Set("org_id", strconv.FormatInt(d.OrganizationID, 10))
	}
	if d.BranchID > 0 {
		v.Set("branch_id", strconv.FormatInt(d.BranchID, 10))
	}
	if d.WarehouseID > 0 {
		v.Set("warehouse_id", strconv.FormatInt(d.WarehouseID, 10))
	}
	if d.StockFilter != "" {
		v.Set("stock", d.StockFilter)
	}
	if d.ExpiringSoon {
		v.Set("expiring", "1")
	}
	if d.PerPage > 0 {
		v.Set("limit", strconv.Itoa(d.PerPage))
	}
	return v
}
