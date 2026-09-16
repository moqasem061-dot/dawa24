package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// إدارة الشحنات — the delivery representative's portal, at /vendor/delivery.
//
// It replaces the unlisted /delivery page a supplier used to share as a link.
// That page authenticated nobody: it took a waybill number in a query string
// and would show anyone holding it the pharmacy's address, the cash to
// collect, and a form that closed the order. This one is a page on the
// supplier's own dashboard, reached by an employee who signed in, showing the
// parcels assigned to that employee and no others.
//
// One page serves two people. A مندوب holds vendor.delivery.view and works
// their own round. A dispatcher also holds vendor.delivery.assign and sees the
// unassigned pool and the whole team's board. Which tabs exist is decided by
// that permission, and the queue the service reads is decided by it again —
// the tab is a request, not an authority.

// VendorDeliveryPortalPage renders the dispatch board.
func (h *UIHandler) VendorDeliveryPortalPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/delivery", http.StatusSeeOther)
		return
	}
	if h.commSvc == nil {
		h.renderPage(ctx, w, "render delivery portal fallback",
			pages.VendorDeliveryPage(pages.VendorDeliveryData{}, lang, dir))
		return
	}

	canAssign := actor.Can("vendor.delivery.assign")
	queue := commerce.ParseCourierQueue(r.URL.Query().Get("queue"))
	// A dispatch tab requested by someone who may not dispatch is a stale link
	// or a hand-typed URL. Falling back to their own round is the honest
	// answer: they are not forbidden from the page, only from that view of it.
	if queue.IsDispatch() && !canAssign {
		queue = commerce.CourierQueueMine
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	search := strings.TrimSpace(r.URL.Query().Get("q"))

	shipments, total, err := h.commSvc.ListCourierQueue(ctx, commerce.CourierQueueFilter{
		VendorOrgID:   actor.OrganizationID,
		CourierUserID: actor.UserID,
		Queue:         queue,
		Search:        search,
		Limit:         limit,
		Offset:        (page - 1) * limit,
	})
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	// The tab badges. Rendering them as zero after a failure would tell a
	// courier they have nothing to deliver, which is the one wrong answer this
	// screen can give.
	counts, err := h.commSvc.CourierQueueCounts(ctx, actor.OrganizationID, actor.UserID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	noticeType, noticeMsg := noticeFrom(r)
	data := pages.VendorDeliveryData{
		Queue:      queue,
		Shipments:  shipments,
		Counts:     counts,
		Search:     search,
		Page:       page,
		PerPage:    limit,
		TotalCount: total,
		CanAssign:  canAssign,
		ActorID:    actor.UserID,
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
	}

	// The round, for the header banner. It is planned only when the caller is
	// actually carrying something — a dispatcher who delivers nothing should
	// not pay for a routing query on every page view — and a failure to plan
	// it is not a failure of the board: the banner falls back to an invitation
	// without numbers rather than taking the whole page down.
	if counts.Mine > 0 {
		if route, rErr := h.commSvc.PlanCourierRoute(ctx, actor.OrganizationID, actor.UserID,
			h.warehouseOrigin(ctx, actor)); rErr == nil {
			data.Route = route
		} else {
			h.log.WarnContext(ctx, "delivery board: route summary unavailable",
				"error", rErr, "user_id", actor.UserID)
		}
	}

	// The two dispatcher-only panels: who is carrying what, and who may be
	// handed something. They are read only for the caller who can act on them.
	if canAssign {
		couriers, cErr := h.listDeliveryCouriers(ctx, actor.OrganizationID)
		if cErr != nil {
			h.renderError(w, r, cErr)
			return
		}
		workload, wErr := h.commSvc.ListCourierWorkload(ctx, actor.OrganizationID)
		if wErr != nil {
			h.renderError(w, r, wErr)
			return
		}
		data.Couriers, data.Workload = couriers, workload
	}

	h.renderPage(ctx, w, "render vendor delivery portal", pages.VendorDeliveryPage(data, lang, dir))
}

// VendorDeliveryShipmentPage renders one parcel's working screen: where it is
// going, who signs for it, what to collect, what is inside, and the actions
// that close it.
func (h *UIHandler) VendorDeliveryShipmentPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/delivery", http.StatusSeeOther)
		return
	}
	shipmentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || shipmentID <= 0 || h.commSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/delivery", "error",
			i18n.T(langOf(r), "vendor.delivery.shipment_not_found"))
		return
	}

	shipment, err := h.commSvc.GetCourierShipment(ctx, shipmentID, actor.OrganizationID,
		deliveryOwnershipFor(actor))
	if err != nil {
		h.log.WarnContext(ctx, "delivery portal: open shipment refused",
			"error", err, "shipment_id", shipmentID, "user_id", actor.UserID)
		h.redirectWithNotice(w, r, "/vendor/delivery", "error", h.safeMessage(err, langOf(r)))
		return
	}

	noticeType, noticeMsg := noticeFrom(r)
	data := pages.VendorDeliveryDetailData{
		Shipment:   shipment,
		CanAssign:  actor.Can("vendor.delivery.assign"),
		CanUpdate:  actor.Can("vendor.delivery.update") && shipment.IsAssignedTo(actor.UserID),
		IsMine:     shipment.IsAssignedTo(actor.UserID),
		ActorID:    actor.UserID,
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
	}
	if data.CanAssign {
		couriers, cErr := h.listDeliveryCouriers(ctx, actor.OrganizationID)
		if cErr != nil {
			h.renderError(w, r, cErr)
			return
		}
		data.Couriers = couriers
	}

	h.renderPage(ctx, w, "render vendor delivery shipment", pages.VendorDeliveryDetailPage(data, lang, dir))
}

// VendorDeliveryStatusSubmit advances a parcel on the caller's own round —
// picked up, in transit, at the door. Handover is not one of these: it needs
// the pharmacy's code and goes through VendorDeliveryVerifySubmit.
func (h *UIHandler) VendorDeliveryStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/delivery", http.StatusSeeOther)
		return
	}
	shipmentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || shipmentID <= 0 || h.commSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/delivery", "error",
			i18n.T(langOf(r), "vendor.delivery.shipment_not_found"))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/delivery", "error",
			i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}

	back := fmt.Sprintf("/vendor/delivery/%d", shipmentID)
	to := commerce.OrderStatus(strings.TrimSpace(r.PostFormValue("status")))
	notes := strings.TrimSpace(r.PostFormValue("notes"))
	failureReason := strings.TrimSpace(r.PostFormValue("failure_reason"))

	if to == commerce.StatusFailed {
		if failureReason == "" && notes == "" {
			h.redirectWithNotice(w, r, back, "error", "يجب تحديد وتوضيح سبب تعذر التسليم لتسجيل الواقعة مع الطلب.")
			return
		}
		if failureReason != "" && notes != "" {
			notes = fmt.Sprintf("سبب التعذر: %s — تفاصيل: %s", failureReason, notes)
		} else if failureReason != "" {
			notes = fmt.Sprintf("سبب التعذر: %s", failureReason)
		}
	}

	shipment, err := h.commSvc.AdvanceCourierShipment(ctx, shipmentID, actor.OrganizationID, actor.UserID, to, notes)
	if err != nil {
		h.log.WarnContext(ctx, "delivery portal: advance shipment failed",
			"error", err, "shipment_id", shipmentID, "to", to, "user_id", actor.UserID)
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.notifyDeliveryProgress(ctx, shipment, actor.OrganizationID)
	h.redirectWithNotice(w, r, back, "success",
		i18n.T(langOf(r), "vendor.delivery.status_updated_success"))
}

// VendorDeliveryVerifySubmit closes a parcel against the pharmacy's six-digit
// code, and tells the buyer and the supplier that it landed.
func (h *UIHandler) VendorDeliveryVerifySubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/delivery", http.StatusSeeOther)
		return
	}
	shipmentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || shipmentID <= 0 || h.commSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/delivery", "error",
			i18n.T(langOf(r), "vendor.delivery.shipment_not_found"))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/delivery", "error",
			i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}

	back := fmt.Sprintf("/vendor/delivery/%d", shipmentID)

	var collectedAmt money.Amount
	hasCollectedAmt := false
	if rawAmt := strings.TrimSpace(r.PostFormValue("collected_amount")); rawAmt != "" {
		if parsed, errParse := money.Parse(rawAmt); errParse == nil {
			collectedAmt = parsed
			hasCollectedAmt = true
		}
	}

	var completed *commerce.OrderShipment
	if hasCollectedAmt {
		completed, err = h.commSvc.CompleteCourierDelivery(ctx, shipmentID, actor.OrganizationID, actor.UserID,
			r.PostFormValue("delivery_code"), r.PostFormValue("notes"), collectedAmt)
	} else {
		completed, err = h.commSvc.CompleteCourierDelivery(ctx, shipmentID, actor.OrganizationID, actor.UserID,
			r.PostFormValue("delivery_code"), r.PostFormValue("notes"))
	}
	if err != nil {
		h.log.WarnContext(ctx, "delivery portal: handover verification failed",
			"error", err, "shipment_id", shipmentID, "user_id", actor.UserID)
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, langOf(r)))
		return
	}

	// Automatically record collected cash as an invoice payment
	if h.billSvc != nil && completed != nil && collectedAmt.IsPositive() {
		inv, errGet := h.billSvc.GetInvoiceByOrderID(ctx, completed.OrderID)
		if errGet != nil || inv == nil {
			if completedOrder, errOrd := h.commSvc.GetOrder(ctx, completed.OrderID); errOrd == nil && completedOrder != nil {
				inv = h.synthesizeInvoiceFromOrder(ctx, completedOrder)
			}
		}
		if inv != nil {
			payReq := billing.RecordInvoicePaymentRequest{
				InvoiceID:       inv.ID,
				OrganizationID:  actor.OrganizationID,
				UserID:          actor.UserID,
				Amount:          collectedAmt,
				Method:          "cash",
				ReferenceNumber: fmt.Sprintf("COLLECT-%s", completed.ShipmentNumber),
				Notes:           fmt.Sprintf("تحصيل نقدي عند التسليم بواسطة المندوب (شحنة %s)", completed.ShipmentNumber),
			}
			if _, errPay := h.billSvc.RecordInvoicePayment(ctx, payReq); errPay != nil {
				h.log.WarnContext(ctx, "failed to record automatic invoice payment from courier delivery",
					"error", errPay, "shipment_id", completed.ID, "invoice_id", inv.ID)
			} else {
				h.log.InfoContext(ctx, "recorded automatic invoice payment from courier delivery",
					"shipment_id", completed.ID, "invoice_id", inv.ID, "amount", collectedAmt.String())
			}
		}
	}

	h.notifyDeliveryCompleted(ctx, completed, actor.OrganizationID)
	h.redirectWithNotice(w, r, back, "success",
		i18n.T(langOf(r), "vendor.delivery.handover_success"))
}

// deliveryOwnershipFor is the "may only open their own parcels" rule, in one
// place. A dispatcher supervises the whole board and passes zero; everyone
// else is confined to what they are carrying.
func deliveryOwnershipFor(actor authctx.Actor) int64 {
	if actor.Can("vendor.delivery.assign") {
		return 0
	}
	return actor.UserID
}

// listDeliveryCouriers resolves the supplier's own delivery representatives —
// the active members whose company role grants the delivery portal.
//
// It asks by permission rather than by role key so a supplier that renamed
// مندوب توصيل, or built its own role for a region, still appears here.
//
// A failure is returned rather than logged and flattened to an empty list. The
// assignment form reads an empty list as "this company has no couriers yet"
// and says so, which after a database error would be a confident lie about the
// company's own team.
func (h *UIHandler) listDeliveryCouriers(ctx context.Context, orgID int64) ([]pages.DeliveryCourierOption, error) {
	if h.orgSvc == nil || orgID <= 0 {
		return nil, nil
	}
	members, err := h.orgSvc.ListMembersHolding(ctx, orgID, "vendor.delivery.view")
	if err != nil {
		return nil, err
	}
	out := make([]pages.DeliveryCourierOption, 0, len(members))
	for _, m := range members {
		if m == nil || m.Member == nil {
			continue
		}
		name := m.UserName
		if name == "" {
			name = m.UserEmail
		}
		out = append(out, pages.DeliveryCourierOption{
			UserID:       m.Member.UserID,
			Name:         name,
			Phone:        m.UserPhone,
			EmployeeCode: m.Member.EmployeeCode,
			BranchName:   m.BranchName,
		})
	}
	return out, nil
}
