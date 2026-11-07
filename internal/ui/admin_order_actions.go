package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
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

	// If a specific shipment is given, transition it; otherwise transition all shipments of the order.
	// A failure is reported, never hidden: the bulk path counts what did not change (B2).
	var failed []error
	targets := order.Shipments
	if shipmentID > 0 {
		targets = nil
		for _, sh := range order.Shipments {
			if sh != nil && sh.ID == shipmentID {
				targets = append(targets, sh)
			}
		}
		if len(targets) == 0 {
			h.redirectWithNotice(w, r, "/admin/orders/"+strconv.FormatInt(orderID, 10), "error", "فشل تغيير حالة الشحنة: الشحنة غير موجودة في هذا الطلب. The shipment is not part of this order.")
			return
		}
	}
	attempted := 0
	for _, sh := range targets {
		if sh == nil {
			continue
		}
		attempted++
		if err := h.adminTransitionShipment(sysCtx, actor.UserID, order, sh, toStatus, notes); err != nil {
			h.logLoadErr(ctx, "commSvc.TransitionShipmentStatus", err)
			failed = append(failed, err)
		}
	}
	if len(failed) > 0 && shipmentID > 0 {
		h.redirectWithNotice(w, r, "/admin/orders/"+strconv.FormatInt(orderID, 10), "error", "فشل تغيير حالة الشحنة: "+failed[0].Error())
		return
	}

	h.safeGo("notify-admin-order-status-changed", func() {
		h.notifyOrderStatusChanged(context.Background(), order, shipmentID, toStatus, vendorName, notes)
	})

	if len(failed) > 0 {
		h.redirectWithNotice(w, r, "/admin/orders/"+strconv.FormatInt(orderID, 10), "error",
			fmt.Sprintf("فشل تغيير حالة %d من %d شحنات: %s. Failed to change %d of %d shipments: %s.",
				len(failed), attempted, failed[0].Error(), len(failed), attempted, failed[0].Error()))
		return
	}

	h.redirectWithNotice(w, r, "/admin/orders/"+strconv.FormatInt(orderID, 10), "success", "تم تحديث حالة الطلب بنجاح وإشعار جميع الأطراف")
}

// adminTransitionShipment moves one shipment as a platform admin. A confirmation of
// a wallet order charges the buyer's wallet in the same transaction and refuses a
// short balance, as a supplier's confirmation does (B2).
func (h *UIHandler) adminTransitionShipment(ctx context.Context, actorID int64, order *commerce.Order, shipment *commerce.OrderShipment, to commerce.OrderStatus, notes string) error {
	// The commerce service checks the shipment against the tenant in its context, so the
	// shipment's own supplier is the tenant. The system marker stays (admin, RLS exempt).
	ctx = database.WithTenant(ctx, shipment.OrganizationID)
	var walletUserID int64
	if to == commerce.StatusConfirmed && shipment.Status != commerce.StatusConfirmed &&
		commerce.IsWalletPayment(order.PaymentMethod) && order.PaymentStatus != commerce.PaymentPaid {
		uid, err := h.prepareWalletShipmentCharge(ctx, order, shipment)
		if err != nil {
			return err
		}
		walletUserID = uid
	}
	_, err := h.commSvc.TransitionShipmentStatusWithWallet(ctx, shipment.ID, to, &actorID, notes, walletUserID)
	return err
}
