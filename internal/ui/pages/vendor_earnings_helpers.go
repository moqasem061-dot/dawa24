package pages

import (
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/ui/components"
)

// VendorEarningsOrderPageData holds the financial summary, paged slices and pagination controls.
type VendorEarningsOrderPageData struct {
	Summary            *commerce.VendorFinancialSummary
	PagedShipments     []*commerce.VendorShipmentProfit
	OrdersPagination   components.PaginationProps
	PagedProducts      []*commerce.VendorProductProfit
	ProductsPagination components.PaginationProps
	CustomerOrgs       []*billing.CustomerOrgSummary
	DateFrom           string
	DateTo             string
	CustomerOrgID      int64
	Lang               string
	Dir                string
}

func vendorEarningsPeriodURL(period string, customerOrgID int64) string {
	if customerOrgID > 0 {
		return fmt.Sprintf("/vendor/earnings/order?period=%s&customer_org_id=%d", period, customerOrgID)
	}
	return fmt.Sprintf("/vendor/earnings/order?period=%s", period)
}
