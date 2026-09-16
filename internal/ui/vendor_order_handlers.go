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
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorOrdersPage renders supplier order fulfillments.
func (h *UIHandler) VendorOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		h.renderPage(ctx, w, "render vendor orders fallback", pages.VendorOrders(pages.VendorOrdersData{}, lang, dir, h.isHTMX(r)))
		return
	}

	filterStatus := strings.TrimSpace(r.URL.Query().Get("status"))
	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	shipments, totalMatching, err := h.commSvc.ListVendorShipmentsWithTotal(ctx, actor.OrganizationID, filterStatus, limit, offset)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	pendingCount, _ := h.commSvc.CountVendorShipmentsByStatus(ctx, actor.OrganizationID, []string{string(commerce.StatusPending)})
	confirmedCount, _ := h.commSvc.CountVendorShipmentsByStatus(ctx, actor.OrganizationID, []string{string(commerce.StatusConfirmed)})
	shippedCount, _ := h.commSvc.CountVendorShipmentsByStatus(ctx, actor.OrganizationID, []string{string(commerce.StatusShipped)})
	deliveredCount, _ := h.commSvc.CountVendorShipmentsByStatus(ctx, actor.OrganizationID, []string{string(commerce.StatusDelivered)})
	failedCount, _ := h.commSvc.CountVendorShipmentsByStatus(ctx, actor.OrganizationID, []string{string(commerce.StatusFailed), string(commerce.StatusReturned)})

	data := pages.VendorOrdersData{
		Shipments:      shipments,
		FilterStatus:   filterStatus,
		Page:           page,
		PerPage:        limit,
		TotalCount:     totalMatching,
		PendingCount:   pendingCount,
		ConfirmedCount: confirmedCount,
		ShippedCount:   shippedCount,
		DeliveredCount: deliveredCount,
		FailedCount:    failedCount,
		CanAssign:      actor.Can("vendor.delivery.assign"),
	}
	// The assignment control on each shipment is rendered from the company's
	// own delivery representatives, and only for a caller who may dispatch.
	if data.CanAssign {
		couriers, cErr := h.listDeliveryCouriers(ctx, actor.OrganizationID)
		if cErr != nil {
			h.renderError(w, r, cErr)
			return
		}
		data.Couriers = couriers
	}

	h.renderPage(ctx, w, "render vendor orders page", pages.VendorOrders(data, lang, dir, h.isHTMX(r)))
}

// VendorOrderStatusSubmit transitions shipment delivery states.
func (h *UIHandler) VendorOrderStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders", http.StatusSeeOther)
		return
	}

	shipmentID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	toStatus := r.PostFormValue("status")
	notes := r.PostFormValue("notes")
	returnTo := r.FormValue("return_to")
	if returnTo == "" || !strings.HasPrefix(returnTo, "/vendor/") {
		returnTo = "/vendor/orders"
	}

	if actor.OrganizationID > 0 {
		ctx = database.WithTenant(ctx, actor.OrganizationID)
	}

	if h.commSvc != nil && shipmentID > 0 && toStatus != "" {
		if err := h.transitionVendorShipment(ctx, actor, shipmentID, commerce.OrderStatus(toStatus), notes,
			strings.TrimSpace(r.PostFormValue("carrier")), strings.TrimSpace(r.PostFormValue("tracking"))); err != nil {
			h.log.ErrorContext(ctx, "vendor transition shipment status failed", "error", err, "shipment", shipmentID, "to", toStatus)
			h.redirectWithNotice(w, r, returnTo, "error", i18n.T(langOf(r), "vendor.orders.update_shipment_status_error_prefix")+h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, returnTo, "success", i18n.T(langOf(r), "vendor.orders.shipment_status_updated_success"))
}

// VendorOrderDetailPage renders the dedicated order and shipment management page for a supplier.
func (h *UIHandler) VendorOrderDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/orders", "error", i18n.T(lang, "orders.order_not_found"))
		return
	}

	if h.commSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/orders", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	shipment, err := h.commSvc.GetVendorShipmentOrByOrderID(ctx, id, actor.OrganizationID)
	if err != nil || shipment == nil {
		h.redirectWithNotice(w, r, "/vendor/orders", "error", i18n.T(lang, "orders.order_not_found"))
		return
	}

	history, _ := h.commSvc.ListOrderHistory(ctx, shipment.OrderID)

	var couriers []pages.DeliveryCourierOption
	canAssign := actor.Can("vendor.delivery.assign")
	if canAssign {
		if cList, cErr := h.listDeliveryCouriers(ctx, actor.OrganizationID); cErr == nil {
			couriers = cList
		}
	}

	noticeType := r.URL.Query().Get("notice")
	noticeMsg := r.URL.Query().Get("msg")

	data := pages.VendorOrderDetailData{
		Shipment:   shipment,
		History:    history,
		CanAssign:  canAssign,
		Couriers:   couriers,
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
		Lang:       lang,
		Dir:        dir,
	}

	h.renderPage(ctx, w, "render vendor order detail", pages.VendorOrderDetail(data))
}

// VendorNegotiationAcceptSubmit accepts a customer's proposed negotiated price and confirms the order.
func (h *UIHandler) VendorNegotiationAcceptSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders", http.StatusSeeOther)
		return
	}

	orderID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	returnTo := r.FormValue("return_to")
	if returnTo == "" || !strings.HasPrefix(returnTo, "/vendor/") {
		returnTo = "/vendor/orders"
	}

	if h.commSvc != nil && orderID > 0 {
		if msg, err := h.decideVendorNegotiation(ctx, actor, langOf(r), orderID, true, ""); msg != "" {
			if err != nil {
				h.log.ErrorContext(ctx, "vendor accept negotiation failed", "error", err, "order_id", orderID)
			}
			h.redirectWithNotice(w, r, returnTo, "error", msg)
			return
		}
	}

	h.redirectWithNotice(w, r, returnTo, "success", i18n.T(langOf(r), "vendor.orders.negotiation_accepted_success"))
}

// VendorNegotiationRejectSubmit rejects a customer's proposed negotiated price and cancels the order.
func (h *UIHandler) VendorNegotiationRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders", http.StatusSeeOther)
		return
	}

	orderID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	returnTo := r.FormValue("return_to")
	if returnTo == "" || !strings.HasPrefix(returnTo, "/vendor/") {
		returnTo = "/vendor/orders"
	}
	reason := r.PostFormValue("reason")
	if reason == "" {
		reason = i18n.T(langOf(r), "vendor.orders.negotiation_default_reject_reason")
	}

	if h.commSvc != nil && orderID > 0 {
		if msg, err := h.decideVendorNegotiation(ctx, actor, langOf(r), orderID, false, reason); msg != "" {
			if err != nil {
				h.log.ErrorContext(ctx, "vendor reject negotiation failed", "error", err, "order_id", orderID)
			}
			h.redirectWithNotice(w, r, returnTo, "error", msg)
			return
		}
	}

	h.redirectWithNotice(w, r, returnTo, "success", i18n.T(langOf(r), "vendor.orders.negotiation_rejected_success"))
}

// transitionVendorShipment moves a supplier's shipment to a new status, records
// tracking, and tells the buyer. The commerce service refuses a shipment of
// another organisation and a transition outside the status machine.
func (h *UIHandler) transitionVendorShipment(
	ctx context.Context, actor authctx.Actor, shipmentID int64, to commerce.OrderStatus, notes, carrier, tracking string,
) error {
	shipment, sErr := h.commSvc.GetShipment(database.AsSystem(ctx), shipmentID)
	if sErr != nil || shipment == nil {
		return fmt.Errorf("shipment not found")
	}
	order, oErr := h.commSvc.GetOrder(database.AsSystem(ctx), shipment.OrderID)
	if oErr != nil || order == nil {
		return fmt.Errorf("order not found")
	}

	var walletDebited bool
	if to == commerce.StatusConfirmed && order.PaymentMethod == "wallet" && order.PaymentStatus != commerce.PaymentPaid {
		if err := h.executeOrderWalletPayment(ctx, order, shipment); err != nil {
			return err
		}
		walletDebited = true
	}

	if _, err := h.commSvc.TransitionShipmentStatus(ctx, shipmentID, to, &actor.UserID, notes); err != nil {
		if walletDebited {
			_ = h.refundOrderWalletPayment(ctx, order, shipment, "فشل اعتماد حالة الشحنة")
		}
		return err
	}

	if to == commerce.StatusCancelled && order.PaymentMethod == "wallet" && order.PaymentStatus == commerce.PaymentPaid {
		_ = h.refundOrderWalletPayment(ctx, order, shipment, "إلغاء أمر التوريد من المورد")
	}

	if to == commerce.StatusShipped && tracking == "" {
		tracking = commerce.GenerateTrackingNumber(fmt.Sprintf("%d", shipmentID), 1)
	}
	if carrier != "" || tracking != "" {
		_ = h.commSvc.SetShipmentTracking(ctx, shipmentID, carrier, tracking)
	}
	vendorName := h.resolveOrgName(ctx, actor.OrganizationID)
	h.safeGo("notify-order-status-changed", func() {
		h.notifyOrderStatusChanged(context.Background(), order, shipmentID, to, vendorName, notes)
	})
	return nil
}

// supplierOrder loads an order this supplier ships part of.
func (h *UIHandler) supplierOrder(ctx context.Context, actor authctx.Actor, lang string, orderID int64) (*commerce.Order, string) {
	order, err := h.commSvc.GetOrder(ctx, orderID)
	if err != nil || order == nil {
		return nil, i18n.T(lang, "vendor.orders.order_not_found")
	}
	for _, sh := range order.Shipments {
		if sh != nil && sh.OrganizationID == actor.OrganizationID {
			return order, ""
		}
	}
	return nil, i18n.T(lang, "vendor.orders.unauthorized_order_management")
}

// decideVendorNegotiation accepts or rejects a buyer's proposed prices and
// tells the buyer. It returns a user-facing message on failure, with the
// underlying error when there is one to log.
func (h *UIHandler) decideVendorNegotiation(
	ctx context.Context, actor authctx.Actor, lang string, orderID int64, accept bool, reason string,
) (string, error) {
	order, msg := h.supplierOrder(ctx, actor, lang, orderID)
	if msg != "" {
		return msg, nil
	}
	if accept {
		var walletDebited bool
		if order.PaymentMethod == "wallet" && order.PaymentStatus != commerce.PaymentPaid {
			if err := h.executeOrderWalletPayment(ctx, order, nil); err != nil {
				return "تعذر قبول التفاوض: " + err.Error(), err
			}
			walletDebited = true
		}
		if err := h.commSvc.AcceptNegotiation(ctx, orderID, actor.UserID); err != nil {
			if walletDebited {
				_ = h.refundOrderWalletPayment(ctx, order, nil, "فشل اعتماد التفاوض")
			}
			return i18n.T(lang, "vendor.orders.accept_negotiation_error_prefix") + h.safeMessage(err, lang), err
		}
	} else if err := h.commSvc.RejectNegotiation(ctx, orderID, reason, actor.UserID); err != nil {
		return i18n.T(lang, "vendor.orders.reject_negotiation_error_prefix") + h.safeMessage(err, lang), err
	}

	vendorName := h.resolveOrgName(ctx, actor.OrganizationID)
	orderNum := order.OrderNumber
	if orderNum == "" {
		orderNum = fmt.Sprintf("ORD-%d", order.ID)
	}
	var custOrgID int64
	if order.OrganizationID != nil {
		custOrgID = *order.OrganizationID
	}
	h.safeGo("notify-negotiation-decision", func() {
		h.notifyNegotiationDecision(context.Background(), order.CustomerID, custOrgID, vendorName, orderNum, accept, reason)
	})
	return "", nil
}
