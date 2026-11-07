package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// AdminConfirmDeliveryOverride confirms a parcel as delivered for a buyer who
// cannot give the PIN. The route needs commerce.delivery.override, the reason is
// required, and the commerce service writes the audit log entry.
func (h *UIHandler) AdminConfirmDeliveryOverride(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/orders", http.StatusSeeOther)
		return
	}
	orderID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	shipmentID, _ := strconv.ParseInt(chi.URLParam(r, "shipmentID"), 10, 64)
	back := fmt.Sprintf("/admin/orders/%d", orderID)
	if orderID <= 0 || shipmentID <= 0 || h.commSvc == nil {
		h.redirectWithNotice(w, r, "/admin/orders", "error", "معرف الطلب أو الشحنة غير صحيح. Invalid order or shipment.")
		return
	}
	_ = r.ParseForm()
	reason := strings.TrimSpace(r.PostFormValue("reason"))

	sysCtx := database.AsSystem(ctx)
	shipment, err := h.commSvc.GetShipment(sysCtx, shipmentID)
	if err != nil || shipment == nil || shipment.OrderID != orderID {
		h.redirectWithNotice(w, r, back, "error", "الشحنة غير موجودة في هذا الطلب. The shipment is not part of this order.")
		return
	}
	// The commerce service checks the parcel against the tenant in its context,
	// so the parcel's own supplier is the tenant, as in the admin status path.
	tenantCtx := database.WithTenant(sysCtx, shipment.OrganizationID)
	completed, err := h.commSvc.OverrideShipmentDelivery(tenantCtx, shipmentID, actor.UserID, reason)
	if err != nil {
		h.log.WarnContext(ctx, "admin delivery override failed", "error", err, "shipment_id", shipmentID, "actor_user_id", actor.UserID)
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.notifyDeliveryCompleted(ctx, completed, shipment.OrganizationID)
	h.redirectWithNotice(w, r, back, "success", "تم تأكيد التسليم وتسجيل الإجراء. Delivery confirmed and logged.")
}
