package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// The supplier's half of the delivery feature: handing a parcel to a مندوب.
//
// Assignment is reachable from two screens because a supplier plans a round
// from two places — أوامر التوريد, where the parcel was created, and إدارة
// الشحنات, where the board is. Both post here and both come back to where they
// started, so the redirect target is a form field rather than a second handler.

// VendorDeliveryAssignSubmit puts one parcel on a delivery representative's
// round and tells them it is waiting.
func (h *UIHandler) VendorDeliveryAssignSubmit(w http.ResponseWriter, r *http.Request) {
	h.submitCourierAssignment(w, r, true)
}

// VendorDeliveryUnassignSubmit takes a parcel back into the unassigned pool.
func (h *UIHandler) VendorDeliveryUnassignSubmit(w http.ResponseWriter, r *http.Request) {
	h.submitCourierAssignment(w, r, false)
}

func (h *UIHandler) submitCourierAssignment(w http.ResponseWriter, r *http.Request, assigning bool) {
	ctx := r.Context()
	lang := langOf(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/delivery", http.StatusSeeOther)
		return
	}
	shipmentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || shipmentID <= 0 || h.commSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/delivery", "error",
			i18n.T(lang, "vendor.delivery.shipment_not_found"))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/delivery", "error",
			i18n.T(lang, "common.invalid_form_data"))
		return
	}
	back := courierAssignmentReturnTo(r)

	sh, getErr := h.commSvc.GetShipment(database.AsSystem(ctx), shipmentID)
	if getErr != nil || sh == nil || sh.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, back, "error",
			i18n.T(lang, "vendor.delivery.shipment_not_found"))
		return
	}
	if sh.Status == commerce.StatusPending {
		h.redirectWithNotice(w, r, back, "error",
			"يجب قبول وتأكيد أمر التوريد أولاً قبل إسناد المندوب أو بدء إجراءات الشحن.")
		return
	}

	var courierUserID *int64
	if assigning {
		id, parseErr := strconv.ParseInt(strings.TrimSpace(r.PostFormValue("courier_user_id")), 10, 64)
		if parseErr != nil || id <= 0 {
			h.redirectWithNotice(w, r, back, "error",
				i18n.T(lang, "vendor.delivery.choose_courier"))
			return
		}
		// The dropdown is rendered from this same list, so a value outside it
		// is a forged form. Re-resolving rather than trusting the post is what
		// stops a supplier assigning a parcel to somebody else's employee.
		isCourier, lookupErr := h.isDeliveryCourier(ctx, actor.OrganizationID, id)
		if lookupErr != nil {
			// Refusing here would tell a dispatcher that their own employee is
			// not a courier, which is a different and wronger thing to say than
			// "the lookup failed". Report the failure as itself.
			h.log.ErrorContext(ctx, "delivery assignment: courier lookup failed",
				"error", lookupErr, "organization_id", actor.OrganizationID)
			h.redirectWithNotice(w, r, back, "error", h.safeMessage(lookupErr, lang))
			return
		}
		if !isCourier {
			h.log.WarnContext(ctx, "delivery assignment refused: not a courier of this company",
				"organization_id", actor.OrganizationID, "courier_user_id", id, "actor", actor.UserID)
			h.redirectWithNotice(w, r, back, "error",
				i18n.T(lang, "vendor.delivery.not_a_courier"))
			return
		}
		courierUserID = &id
	}

	shipment, err := h.commSvc.AssignShipmentToCourier(ctx, shipmentID, actor.OrganizationID, courierUserID, actor.UserID)
	if err != nil {
		h.log.ErrorContext(ctx, "delivery assignment failed",
			"error", err, "shipment_id", shipmentID, "organization_id", actor.OrganizationID)
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, lang))
		return
	}

	if assigning && courierUserID != nil {
		h.notifyCourierAssigned(ctx, shipment, *courierUserID)
		h.redirectWithNotice(w, r, back, "success",
			fmt.Sprintf(i18n.T(lang, "vendor.delivery.assigned_success"), shipment.ShipmentNumber))
		return
	}
	h.redirectWithNotice(w, r, back, "success",
		i18n.T(lang, "vendor.delivery.unassigned_success"))
}

// courierAssignmentReturnTo keeps the dispatcher on the screen they acted
// from. Only the two screens that carry the form are accepted, so the field
// cannot be turned into an open redirect.
func courierAssignmentReturnTo(r *http.Request) string {
	ret := strings.TrimSpace(r.PostFormValue("return_to"))
	switch ret {
	case "orders":
		return "/vendor/orders"
	case "board":
		return "/vendor/delivery?queue=" + string(commerce.CourierQueueAll)
	case "unassigned":
		return "/vendor/delivery?queue=" + string(commerce.CourierQueueUnassigned)
	}
	if strings.HasPrefix(ret, "/vendor/orders") {
		return ret
	}
	if strings.HasPrefix(r.URL.Path, "/vendor/orders/") {
		if id := chi.URLParam(r, "id"); id != "" {
			return "/vendor/orders/" + id
		}
	}
	if id := chi.URLParam(r, "id"); id != "" {
		return "/vendor/delivery/" + id
	}
	return "/vendor/delivery"
}

// isDeliveryCourier reports whether a user is one of this supplier's own
// delivery representatives.
func (h *UIHandler) isDeliveryCourier(ctx context.Context, orgID, userID int64) (bool, error) {
	couriers, err := h.listDeliveryCouriers(ctx, orgID)
	if err != nil {
		return false, err
	}
	for _, c := range couriers {
		if c.UserID == userID {
			return true, nil
		}
	}
	return false, nil
}

// notifyCourierAssigned tells a مندوب that a parcel is waiting for them, with
// the two facts they need to plan around it: which pharmacy, and how much is
// in the parcel.
func (h *UIHandler) notifyCourierAssigned(ctx context.Context, shipment *commerce.OrderShipment, courierUserID int64) {
	if shipment == nil || courierUserID <= 0 {
		return
	}
	pharmacy := shipment.CustomerOrgName.Get(i18n.AR)
	if pharmacy == "" {
		pharmacy = shipment.CustomerOrgName.Get(i18n.EN)
	}
	h.safeGo("dispatch-courier-delivery-notif", func() {
		h.dispatchInAppNotification(context.WithoutCancel(ctx), courierUserID, nil, "vendor.delivery.view",
			fmt.Sprintf(i18n.TDefault("vendor.delivery.courier_notif_title"), shipment.ShipmentNumber),
			fmt.Sprintf(i18n.TDefault("vendor.delivery.courier_notif_body"), shipment.ShipmentNumber, pharmacy),
		)
	})
}

// notifyDeliveryProgress tells the buying pharmacy that their parcel moved.
func (h *UIHandler) notifyDeliveryProgress(ctx context.Context, shipment *commerce.OrderShipment, orgID int64) {
	if shipment == nil || h.commSvc == nil {
		return
	}
	order, err := h.commSvc.GetOrder(database.AsSystem(ctx), shipment.OrderID)
	if err != nil || order == nil {
		h.log.WarnContext(ctx, "delivery progress notification: order not found",
			"error", err, "order_id", shipment.OrderID)
		return
	}
	vendorName := h.resolveOrgName(ctx, orgID)
	h.safeGo("notify-order-status-changed", func() {
		h.notifyOrderStatusChanged(context.WithoutCancel(ctx), order, shipment.ID,
			shipment.Status, vendorName, shipment.DeliveryNotes)
	})
}

// notifyDeliveryCompleted tells the pharmacy and the supplier that the parcel
// was signed for.
func (h *UIHandler) notifyDeliveryCompleted(ctx context.Context, shipment *commerce.OrderShipment, orgID int64) {
	if shipment == nil || h.commSvc == nil {
		return
	}
	order, err := h.commSvc.GetOrder(database.AsSystem(ctx), shipment.OrderID)
	if err != nil || order == nil {
		h.log.WarnContext(ctx, "delivery completion notification: order not found",
			"error", err, "order_id", shipment.OrderID)
		return
	}
	orderNum := order.OrderNumber
	if orderNum == "" {
		orderNum = fmt.Sprintf("ORD-%d", order.ID)
	}
	vendorName := h.resolveOrgName(ctx, orgID)
	notifyCtx := context.WithoutCancel(ctx)
	vendorOrg := orgID

	h.safeGo("dispatch-delivery-customer-notif", func() {
		h.dispatchInAppBranchNotification(notifyCtx, order.CustomerID, order.OrganizationID, order.BranchID, "pharmacy.order.view",
			fmt.Sprintf(i18n.TDefault("courier.customer_notif_title"), shipment.ShipmentNumber),
			fmt.Sprintf(i18n.TDefault("courier.customer_notif_body"), orderNum, vendorName),
		)
	})
	h.safeGo("dispatch-delivery-vendor-notif", func() {
		h.dispatchOrgBranchNotification(notifyCtx, vendorOrg, shipment.BranchID, "vendor.delivery.view",
			fmt.Sprintf(i18n.TDefault("courier.vendor_notif_title"), shipment.ShipmentNumber),
			fmt.Sprintf(i18n.TDefault("courier.vendor_notif_body"), shipment.ShipmentNumber),
		)
	})
}
