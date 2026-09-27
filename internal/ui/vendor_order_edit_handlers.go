package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// VendorOrderEditSubmit lets a supplier change quantities or remove lines of
// its own shipment before accepting it. The {id} is the shipment id, as on
// the detail page. Totals are recomputed server-side from the stored prices;
// the wallet is only debited on acceptance, so it charges the edited total.
func (h *UIHandler) VendorOrderEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders", http.StatusSeeOther)
		return
	}

	shipmentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || shipmentID <= 0 || h.commSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/orders", "error", i18n.T(lang, "orders.order_not_found"))
		return
	}
	back := fmt.Sprintf("/vendor/orders/%d", shipmentID)

	shipment, err := h.commSvc.GetVendorShipment(ctx, shipmentID, actor.OrganizationID)
	if err != nil || shipment == nil {
		h.redirectWithNotice(w, r, "/vendor/orders", "error", i18n.T(lang, "orders.order_not_found"))
		return
	}

	updated, err := h.commSvc.UpdateVendorPendingShipment(ctx, actor, commerce.UpdateCustomerOrderInput{
		OrderID: shipment.OrderID,
		Lines:   parseOrderLineEdits(r),
	})
	if err != nil {
		h.log.WarnContext(ctx, "vendor order edit failed", "error", err, "shipment_id", shipmentID)
		h.redirectWithNotice(w, r, back, "error",
			fmt.Sprintf(i18n.T(lang, "vendor.order.edit_error"), h.safeMessage(err, lang)))
		return
	}

	if updated != nil && updated.OrganizationID != nil && *updated.OrganizationID > 0 {
		buyerOrgID, buyerBranchID := *updated.OrganizationID, updated.BranchID
		orderNum := updated.OrderNumber
		if orderNum == "" {
			orderNum = fmt.Sprintf("ORD-%d", updated.ID)
		}
		vendorName := h.resolveOrgName(ctx, actor.OrganizationID)
		title := fmt.Sprintf(i18n.T("ar", "vendor.order.edit_notification_title"), orderNum)
		body := fmt.Sprintf(i18n.T("ar", "vendor.order.edit_notification_body"), vendorName, orderNum, updated.TotalAmount.String())
		h.safeGo("dispatch-vendor-order-edit-notif", func() {
			h.dispatchOrgBranchNotification(context.Background(), buyerOrgID, buyerBranchID, "pharmacy.order.view", title, body)
		})
	}

	h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "vendor.order.edit_success"))
}
