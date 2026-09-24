package ui

import (
	"context"
	"fmt"
	"strconv"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/notifications"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// AnnounceOrderPlaced is the commerce.OrderPlacedHook every commerce service
// is wired with, so an order is announced once whichever path placed it: the
// checkout page, Capsule (browser or Telegram), negotiation, Smart Ordering or
// the JSON API. It returns at once; fan-out runs in the background.
func (h *UIHandler) AnnounceOrderPlaced(ctx context.Context, order *commerce.Order) {
	if order == nil {
		return
	}
	h.safeGo("notify-order-placed", func() {
		buyerName := ""
		if order.OrganizationID != nil {
			buyerName = h.resolveOrgName(ctx, *order.OrganizationID)
		}
		h.notifyOrderPlaced(ctx, order, buyerName)
	})
}

// notifyOrderPlaced tells the three parties to a new order.
//
//   - The buyer: the person who placed it, and the members of the buying
//     company who may see its purchases, on the receiving branch. A supplier
//     buying for itself is a buyer like any pharmacy; the permission is scoped
//     to its dashboard by dispatch (vendor.buying.order.view).
//   - Each fulfilling supplier's order team, on the fulfilling branch. A
//     negotiation is announced to the supplier as a negotiation offer by the
//     negotiation form instead, so it is not told twice about one order.
//   - Platform staff who may see orders.
func (h *UIHandler) notifyOrderPlaced(ctx context.Context, order *commerce.Order, buyerName string) {
	if order == nil {
		return
	}

	orderNum := order.OrderNumber
	if orderNum == "" {
		orderNum = fmt.Sprintf("ORD-%d", order.ID)
	}

	// 1. Buyer
	custVars := map[string]string{
		"order_number": orderNum,
		"total_amount": order.TotalAmount.String(),
	}
	h.dispatchEvent(ctx, notifications.EventOrderPlaced, order.CustomerID, order.OrganizationID, custVars)
	if order.OrganizationID != nil && *order.OrganizationID > 0 {
		h.dispatchOrgBranchEvent(ctx, notifications.EventOrderPlaced, *order.OrganizationID, order.BranchID, custVars)
	}

	if buyerName == "" {
		buyerName = i18n.T("ar", "notif.verified_pharmacy")
	}

	// 2. Suppliers
	if !order.IsNegotiation {
		for _, sh := range order.Shipments {
			if sh == nil || sh.OrganizationID <= 0 {
				continue
			}
			vendorTitle := fmt.Sprintf(i18n.T("ar", "notif.new_supply_order_title"), orderNum)
			vendorBody := fmt.Sprintf(i18n.T("ar", "notif.new_supply_order_body"),
				buyerName, len(sh.Lines), sh.Subtotal.String())
			vendorBranch := sh.BranchID
			if vendorBranch == nil {
				vendorBranch = order.VendorBranchID
			}
			h.dispatchOrgBranchNotification(ctx, sh.OrganizationID, vendorBranch, "vendor.order.view", vendorTitle, vendorBody)
		}
	}

	// 3. Platform
	h.dispatchAdminEvent(ctx, notifications.EventAdminOrderPlaced, map[string]string{
		"order_number":   orderNum,
		"customer_name":  buyerName,
		"account_type":   h.buyerTypeLabel(ctx, order.OrganizationID),
		"total_amount":   order.TotalAmount.String(),
		"shipment_count": strconv.Itoa(len(order.Shipments)),
	})
}

// buyerTypeLabel names the kind of company that bought, for the platform's
// copy: a supplier restocking reads differently from a pharmacy ordering.
func (h *UIHandler) buyerTypeLabel(ctx context.Context, orgID *int64) string {
	if orgID != nil && *orgID > 0 && h.orgSvc != nil {
		if o, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), *orgID); err == nil && o != nil {
			if s, ok := rbac.TenantScopeFor(string(o.Type)); ok && s == rbac.ScopeVendor {
				return "مورد"
			}
		}
	}
	return "صيدلية"
}
