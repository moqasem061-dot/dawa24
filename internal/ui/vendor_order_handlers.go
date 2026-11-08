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
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/pagination"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
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

	pendingCount, loadErr := h.commSvc.CountVendorShipmentsByStatus(ctx, actor.OrganizationID, []string{string(commerce.StatusPending)})
	h.logLoadErr(ctx, "commSvc.CountVendorShipmentsByStatus", loadErr)
	confirmedCount, loadErr := h.commSvc.CountVendorShipmentsByStatus(ctx, actor.OrganizationID, []string{string(commerce.StatusConfirmed)})
	h.logLoadErr(ctx, "commSvc.CountVendorShipmentsByStatus", loadErr)
	shippedCount, loadErr := h.commSvc.CountVendorShipmentsByStatus(ctx, actor.OrganizationID, []string{string(commerce.StatusShipped)})
	h.logLoadErr(ctx, "commSvc.CountVendorShipmentsByStatus", loadErr)
	deliveredCount, loadErr := h.commSvc.CountVendorShipmentsByStatus(ctx, actor.OrganizationID, []string{string(commerce.StatusDelivered)})
	h.logLoadErr(ctx, "commSvc.CountVendorShipmentsByStatus", loadErr)
	failedCount, loadErr := h.commSvc.CountVendorShipmentsByStatus(ctx, actor.OrganizationID, []string{string(commerce.StatusFailed), string(commerce.StatusReturned)})
	h.logLoadErr(ctx, "commSvc.CountVendorShipmentsByStatus", loadErr)

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

	// A delivery closed with the buyer's PIN goes through the PIN handover.
	if code := strings.TrimSpace(r.PostFormValue("delivery_code")); code != "" && toStatus == string(commerce.StatusDelivered) && h.commSvc != nil && shipmentID > 0 {
		h.vendorDeliverWithPIN(w, r, ctx, actor, shipmentID, code, notes, returnTo)
		return
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

	history, loadErr := h.commSvc.ListOrderHistory(ctx, shipment.OrderID)
	h.logLoadErr(ctx, "commSvc.ListOrderHistory", loadErr)

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
		// The buyer branch phone is masked in the copy the page renders
		// (plan.md S8, FR-032); the reveal control names the order.
		Shipment:   maskedShipment(shipment),
		CanReveal:  actor.Can(revealPermSupplier),
		History:    history,
		CanAssign:  canAssign,
		CanEdit:    shipment.Status == commerce.StatusPending && actor.Can("vendor.order.update"),
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
// another organisation and a transition outside the status machine. A confirmation
// of a wallet order charges the shipment to the buyer's wallet in the same
// transaction, so a refused charge leaves the shipment unconfirmed.
func (h *UIHandler) transitionVendorShipment(
	ctx context.Context, actor authctx.Actor, shipmentID int64, to commerce.OrderStatus, notes, carrier, tracking string,
) error {
	vendorName := h.resolveOrgName(ctx, actor.OrganizationID)
	return h.transitionVendorShipmentBy(ctx, actor.OrganizationID, actor.IsStaff, &actor.UserID, shipmentID, to, notes, notes, carrier, tracking, vendorName, nil)
}

// transitionVendorShipmentBy holds the transition for the website (changedBy is
// the signed-in user) and for the ERP API (changedBy is nil, a machine has no
// user). Ownership is checked against orgID unless isStaff. notes goes into the
// status history and buyerNote into the buyer's notice; the website passes the
// same text for both. hook, when non-nil, runs inside the transition's own
// transaction (see commerce.ShipmentTxHook).
func (h *UIHandler) transitionVendorShipmentBy(
	ctx context.Context, orgID int64, isStaff bool, changedBy *int64, shipmentID int64, to commerce.OrderStatus,
	notes, buyerNote, carrier, tracking, vendorName string, hook commerce.ShipmentTxHook,
) error {
	shipment, sErr := h.commSvc.GetShipment(database.AsSystem(ctx), shipmentID)
	// Ownership first: the wallet charge below runs before the commerce service
	// would refuse another organisation's shipment.
	if sErr != nil || shipment == nil || (!isStaff && shipment.OrganizationID != orgID) {
		return fmt.Errorf("shipment not found")
	}
	order, oErr := h.commSvc.GetOrder(database.AsSystem(ctx), shipment.OrderID)
	if oErr != nil || order == nil {
		return fmt.Errorf("order not found")
	}

	var walletUserID int64
	if to == commerce.StatusConfirmed && shipment.Status != commerce.StatusConfirmed &&
		commerce.IsWalletPayment(order.PaymentMethod) && order.PaymentStatus != commerce.PaymentPaid {
		uid, err := h.prepareWalletShipmentCharge(ctx, order, shipment)
		if err != nil {
			return err
		}
		walletUserID = uid
	}

	// A confirmation charges the wallet and a cancel refunds it, inside
	// TransitionShipmentStatusWithWallet, in the same transaction as the status.
	if _, err := h.commSvc.TransitionShipmentStatusWithWallet(ctx, shipmentID, to, changedBy, notes, walletUserID, hook); err != nil {
		return err
	}

	if to == commerce.StatusShipped && tracking == "" {
		tracking = commerce.GenerateTrackingNumber(fmt.Sprintf("%d", shipmentID), 1)
	}
	if carrier != "" || tracking != "" {
		_ = h.commSvc.SetShipmentTracking(ctx, shipmentID, carrier, tracking)
	}
	h.safeGo("notify-order-status-changed", func() {
		h.notifyOrderStatusChanged(context.Background(), order, shipmentID, to, vendorName, buyerNote)
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
// underlying error when there is one to log. An acceptance confirms this
// supplier's shipments only, and charges them to the buyer's wallet when the
// order is wallet-paid (Batch 2, S02 and S04).
func (h *UIHandler) decideVendorNegotiation(
	ctx context.Context, actor authctx.Actor, lang string, orderID int64, accept bool, reason string,
) (string, error) {
	order, msg := h.supplierOrder(ctx, actor, lang, orderID)
	if msg != "" {
		return msg, nil
	}
	if accept {
		var walletUserID int64
		if commerce.IsWalletPayment(order.PaymentMethod) && order.PaymentStatus != commerce.PaymentPaid {
			uid, err := h.prepareWalletNegotiationCharge(ctx, order, actor.OrganizationID)
			if err != nil {
				return "تعذر قبول التفاوض: " + err.Error(), err
			}
			walletUserID = uid
		}
		if err := h.commSvc.AcceptNegotiation(ctx, orderID, actor.OrganizationID, actor.UserID, walletUserID); err != nil {
			return i18n.T(lang, "vendor.orders.accept_negotiation_error_prefix") + h.safeMessage(err, lang), err
		}
	} else if err := h.commSvc.RejectNegotiation(ctx, orderID, actor.OrganizationID, reason, actor.UserID); err != nil {
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
