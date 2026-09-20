package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// AdminOrderStatusSubmit allows platform admins to transition an order or shipment status.
func (h *UIHandler) AdminOrderStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/orders", http.StatusSeeOther)
		return
	}

	orderID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if orderID <= 0 {
		h.redirectWithNotice(w, r, "/admin/orders", "error", "معرف الطلب غير صحيح")
		return
	}

	_ = r.ParseForm()
	toStatus := commerce.OrderStatus(strings.TrimSpace(r.PostFormValue("status")))
	notes := strings.TrimSpace(r.PostFormValue("notes"))
	shipmentID, _ := strconv.ParseInt(r.PostFormValue("shipment_id"), 10, 64)

	sysCtx := database.AsSystem(ctx)
	order, err := h.commSvc.GetOrder(sysCtx, orderID)
	if err != nil || order == nil {
		h.redirectWithNotice(w, r, "/admin/orders", "error", "لم يتم العثور على الطلب")
		return
	}

	var vendorName string
	if len(order.Shipments) > 0 {
		vendorName = h.resolveOrgName(sysCtx, order.Shipments[0].OrganizationID)
	}

	// If a specific shipment is given, transition it; otherwise transition all shipments of the order
	if shipmentID > 0 {
		if _, err := h.commSvc.TransitionShipmentStatus(sysCtx, shipmentID, toStatus, &actor.UserID, notes); err != nil {
			h.redirectWithNotice(w, r, "/admin/orders/"+strconv.FormatInt(orderID, 10), "error", "فشل تغيير حالة الشحنة: "+err.Error())
			return
		}
	} else {
		for _, sh := range order.Shipments {
			if sh != nil {
				_, _ = h.commSvc.TransitionShipmentStatus(sysCtx, sh.ID, toStatus, &actor.UserID, notes)
			}
		}
	}

	h.safeGo("notify-admin-order-status-changed", func() {
		h.notifyOrderStatusChanged(context.Background(), order, shipmentID, toStatus, vendorName, notes)
	})

	h.redirectWithNotice(w, r, "/admin/orders/"+strconv.FormatInt(orderID, 10), "success", "تم تحديث حالة الطلب بنجاح وإشعار جميع الأطراف")
}
