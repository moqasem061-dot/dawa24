package pages

import (
	"fmt"
	"net/url"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
)

// VendorQuotasData is everything the screen renders.
type VendorQuotasData struct {
	ActiveTab string // "branches" or "variants"
	Summary   commerce.QuotaSummary

	BranchRows  []*commerce.BranchQuotaRow
	VariantRows []*commerce.QuotaVariantRow

	VariantOptions  []commerce.QuotaOption
	CustomerOptions []commerce.QuotaOption
	BranchOptions   []commerce.QuotaOption

	Query         string
	VariantID     int64
	CustomerOrgID int64
	BranchID      int64
	State         string

	Page       int
	PerPage    int
	TotalCount int

	CanManage bool
}

// quotaTabURL keeps the filters attached when the supplier switches tab.
func (d VendorQuotasData) quotaTabURL(tab string) string {
	q := d.filterValues()
	q.Set("tab", tab)
	q.Del("page")
	return "/vendor/quotas?" + q.Encode()
}

// currentURL keeps the active tab, filters, page, and per_page for form return_to actions.
func (d VendorQuotasData) currentURL() string {
	q := d.filterValues()
	q.Set("tab", d.tab())
	if d.Page > 1 {
		q.Set("page", fmt.Sprintf("%d", d.Page))
	}
	if d.PerPage > 0 {
		q.Set("per_page", fmt.Sprintf("%d", d.PerPage))
	}
	return "/vendor/quotas?" + q.Encode()
}

// filterValues is the current filter state as query parameters.
func (d VendorQuotasData) filterValues() url.Values {
	q := url.Values{}
	if d.Query != "" {
		q.Set("q", d.Query)
	}
	if d.VariantID > 0 {
		q.Set("variant", fmt.Sprintf("%d", d.VariantID))
	}
	if d.CustomerOrgID > 0 {
		q.Set("customer", fmt.Sprintf("%d", d.CustomerOrgID))
	}
	if d.BranchID > 0 {
		q.Set("branch", fmt.Sprintf("%d", d.BranchID))
	}
	if d.State != "" {
		q.Set("state", d.State)
	}
	return q
}

// PagerValues is filterValues plus the tab, for components.B2BPagination.
func (d VendorQuotasData) PagerValues() url.Values {
	q := d.filterValues()
	q.Set("tab", d.tab())
	return q
}

func (d VendorQuotasData) tab() string {
	if d.ActiveTab == "variants" {
		return "variants"
	}
	return "branches"
}

// quotaItemLabel names the item a branch consumed, preferring the shared
// catalogue's product name and falling back to the supplier's own variant name.
func quotaItemLabel(row *commerce.BranchQuotaRow) string {
	if row == nil {
		return ""
	}
	return quotaJoinNames(row.ProductName, row.VariantName, row.SKU, row.VariantID)
}

func quotaVariantLabel(row *commerce.QuotaVariantRow) string {
	if row == nil {
		return ""
	}
	return quotaJoinNames(row.ProductName, row.VariantName, row.SKU, row.VariantID)
}

func quotaJoinNames(product, variant, sku string, id int64) string {
	switch {
	case product != "" && variant != "" && product != variant:
		return product + " — " + variant
	case product != "":
		return product
	case variant != "":
		return variant
	case sku != "":
		return sku
	default:
		return fmt.Sprintf("#%d", id)
	}
}

// quotaTimeLabel formats the last order date, or a dash where a released
// pairing has bought nothing since.
func quotaTimeLabel(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format("2006-01-02")
}

func quotaFallback(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
