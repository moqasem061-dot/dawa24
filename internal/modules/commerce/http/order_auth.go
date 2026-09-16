package http

import "github.com/muhiya/dawa24-store/internal/modules/commerce"

// isFulfillingVendor checks whether the target vendor organization is assigned
// to fulfill at least one shipment or line item of the given order.
func isFulfillingVendor(order *commerce.Order, vendorOrgID int64) bool {
	if vendorOrgID <= 0 || order == nil {
		return false
	}
	for _, s := range order.Shipments {
		if s != nil && s.OrganizationID == vendorOrgID {
			return true
		}
	}
	for _, l := range order.Lines {
		if l != nil && l.OrganizationID == vendorOrgID {
			return true
		}
	}
	return false
}
