package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func (h *UIHandler) CustomerOrderEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(r.URL.Path), http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/orders", "error", i18n.T(langOf(r), "customer.order.invalid_id"))
		return
	}

	if h.commSvc == nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "error", i18n.T(langOf(r), "customer.order.service_unavailable"))
		return
	}

	_ = r.ParseForm()

	lineIDs := r.PostForm["line_id[]"]
	productNames := r.PostForm["product_name[]"]
	quantities := r.PostForm["quantity[]"]
	unitPrices := r.PostForm["unit_price[]"]
	discountAmounts := r.PostForm["discount_amount[]"]
	isDeletedList := r.PostForm["is_deleted[]"]

	lineCount := len(lineIDs)
	if len(productNames) > lineCount {
		lineCount = len(productNames)
	}
	if len(quantities) > lineCount {
		lineCount = len(quantities)
	}

	var lines []commerce.OrderLineEditItem
	for i := 0; i < lineCount; i++ {
		var lineID int64
		if i < len(lineIDs) {
			lineID, _ = strconv.ParseInt(lineIDs[i], 10, 64)
		}

		pName := ""
		if i < len(productNames) {
			pName = strings.TrimSpace(productNames[i])
		}

		qty := 1
		if i < len(quantities) {
			if qVal, err := strconv.Atoi(quantities[i]); err == nil && qVal > 0 {
				qty = qVal
			}
		}

		uPrice := money.Zero
		if i < len(unitPrices) {
			if parsed, err := money.Parse(unitPrices[i]); err == nil {
				uPrice = parsed
			}
		}

		dAmount := money.Zero
		if i < len(discountAmounts) {
			if parsed, err := money.Parse(discountAmounts[i]); err == nil {
				dAmount = parsed
			}
		}

		isDel := false
		if i < len(isDeletedList) {
			isDel = isDeletedList[i] == "true" || isDeletedList[i] == "1"
		}

		if lineID <= 0 && pName == "" && !isDel {
			continue
		}

		lines = append(lines, commerce.OrderLineEditItem{
			ID:             lineID,
			ProductName:    pName,
			Quantity:       qty,
			UnitPrice:      uPrice,
			DiscountAmount: dAmount,
			IsDeleted:      isDel,
		})
	}

	input := commerce.UpdateCustomerOrderInput{
		OrderID: id,
		Lines:   lines,
		Notes:   strings.TrimSpace(r.PostFormValue("notes")),
	}

	updatedOrder, err := h.commSvc.UpdateCustomerPendingOrder(ctx, actor, input)
	if err != nil {
		errMsg := fmt.Sprintf(i18n.T(langOf(r), "customer.order.edit_error"), h.safeMessage(err, langOf(r)))
		if r.Header.Get("X-Requested-With") == "XMLHttpRequest" || strings.Contains(r.Header.Get("Accept"), "application/json") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error":   errMsg,
			})
			return
		}
		h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "error", errMsg)
		return
	}

	if updatedOrder != nil {
		pharmacyName := h.resolveOrgName(ctx, actor.OrganizationID)
		orderNum := updatedOrder.OrderNumber
		if orderNum == "" {
			orderNum = fmt.Sprintf("ORD-%d", updatedOrder.ID)
		}
		notifLang := langOf(r)
		for _, sh := range updatedOrder.Shipments {
			if sh != nil && sh.OrganizationID > 0 {
				shOrgID := sh.OrganizationID
				shBranchID := sh.BranchID
				if shBranchID == nil {
					shBranchID = updatedOrder.VendorBranchID
				}
				title := fmt.Sprintf(i18n.T(notifLang, "customer.order.edit_notification_title"), orderNum)
				body := fmt.Sprintf(i18n.T(notifLang, "customer.order.edit_notification_body"), pharmacyName, orderNum, updatedOrder.TotalAmount.String())
				h.safeGo("dispatch-org-order-edit-notif", func() {
					h.dispatchOrgBranchNotification(context.Background(), shOrgID, shBranchID, "vendor.order.view", title, body)
				})
			}
		}

		if updatedOrder.PaymentMethod == "wallet" && h.billSvc != nil {
			newGoodsAmount := updatedOrder.TotalAmount
			if updatedOrder.ShippingFee.IsPositive() && updatedOrder.ShippingFee.Minor() < updatedOrder.TotalAmount.Minor() {
				newGoodsAmount, _ = updatedOrder.TotalAmount.Sub(updatedOrder.ShippingFee)
			}
			walletUserID, _ := resolveTenantUserIDs(ctx, h, actor)
			if wallet, wErr := h.billSvc.GetWallet(ctx, walletUserID, "EGP"); wErr == nil && wallet != nil {
				if wallet.AvailableBalance.Minor() < newGoodsAmount.Minor() {
					h.log.WarnContext(ctx, "customer edited order total exceeds available wallet balance",
						"order_id", updatedOrder.ID, "goods_amount", newGoodsAmount.String(), "available", wallet.AvailableBalance.String())
				}
			}
		}
	}

	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" || strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":     true,
			"message":     i18n.T(langOf(r), "customer.order.edit_success"),
			"subtotal":    updatedOrder.Subtotal.String(),
			"discount":    updatedOrder.DiscountAmount.String(),
			"total":       updatedOrder.TotalAmount.String(),
			"shippingFee": updatedOrder.ShippingFee.String(),
			"taxAmount":   updatedOrder.TaxAmount.String(),
		})
		return
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "success", i18n.T(langOf(r), "customer.order.edit_success"))
}

// CustomerOrderCancelSubmit handles buyer cancellation of an order before shipping/arrival cutoff.
func (h *UIHandler) CustomerOrderCancelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(r.URL.Path), http.StatusSeeOther)
		return
	}

	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/orders", "error", i18n.T(lang, "customer.order.invalid_id"))
		return
	}
	wantsJSON := r.Header.Get("X-Requested-With") == "XMLHttpRequest" || strings.Contains(r.Header.Get("Accept"), "application/json")

	_ = r.ParseForm()
	failure := h.cancelBuyerOrder(ctx, actor, lang, id,
		strings.TrimSpace(r.PostFormValue("reason")), strings.TrimSpace(r.PostFormValue("notes")))
	if failure != nil {
		if failure.JSON && wantsJSON {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": failure.Message})
			return
		}
		h.redirectWithNotice(w, r, failure.Back, "error", failure.Message)
		return
	}

	if wantsJSON {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": i18n.T(lang, "orders.cancel_success_notified"),
		})
		return
	}
	h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "success", i18n.T(lang, "orders.cancel_success_notified"))
}

// orderCancelFailure says why a buyer cancellation was refused. JSON marks the
// refusals the order page's script shows inline rather than by redirect.
type orderCancelFailure struct {
	Back    string
	Message string
	JSON    bool
}

// cancellableBuyerOrder loads an order the buyer may cancel now, or explains
// why not. It changes nothing.
func (h *UIHandler) cancellableBuyerOrder(ctx context.Context, actor authctx.Actor, lang string, id int64) (*commerce.Order, *orderCancelFailure) {
	if h.commSvc == nil {
		return nil, &orderCancelFailure{Back: fmt.Sprintf("/orders/%d", id), Message: i18n.T(lang, "customer.order.service_unavailable")}
	}
	order, err := h.commSvc.GetOrder(ctx, id)
	if err != nil || order == nil {
		return nil, &orderCancelFailure{Back: "/orders", Message: i18n.T(lang, "orders.not_found")}
	}

	// The order must belong to the buyer's current organisation. Matching the
	// placing user alone let a member who switched to another organisation
	// cancel an order that belongs to the first one.
	isOwner := order.OrganizationID != nil && actor.OrganizationID > 0 && *order.OrganizationID == actor.OrganizationID
	if !isOwner && !actor.IsPlatformAdmin() {
		return nil, &orderCancelFailure{Back: fmt.Sprintf("/orders/%d", id), Message: i18n.T(lang, "orders.cancel_unauthorized")}
	}

	switch order.Status {
	case commerce.StatusPending, commerce.StatusProcessing, commerce.StatusConfirmed, commerce.StatusOnHold:
	default:
		msg := fmt.Sprintf(i18n.T(lang, "orders.cancel_invalid_status"), string(order.Status))
		if order.Status == commerce.StatusShipped || order.Status == commerce.StatusInTransit || order.Status == commerce.StatusOutForDelivery || order.Status == commerce.StatusDelivered {
			msg = i18n.T(lang, "orders.cancel_shipment_in_progress")
		}
		return nil, &orderCancelFailure{Back: fmt.Sprintf("/orders/%d", id), Message: msg, JSON: true}
	}

	// Forbid cancellation if any shipment is already on the road or delivered.
	for _, sh := range order.Shipments {
		if sh == nil {
			continue
		}
		switch sh.Status {
		case commerce.StatusShipped, commerce.StatusInTransit, commerce.StatusOutForDelivery, commerce.StatusDelivered, commerce.StatusCompleted:
			return nil, &orderCancelFailure{Back: fmt.Sprintf("/orders/%d", id),
				Message: i18n.T(lang, "orders.cancel_shipment_in_progress_partial"), JSON: true}
		}
	}
	return order, nil
}

// cancelBuyerOrder cancels an order for the buyer and tells its suppliers.
func (h *UIHandler) cancelBuyerOrder(ctx context.Context, actor authctx.Actor, lang string, id int64, reason, notes string) *orderCancelFailure {
	order, failure := h.cancellableBuyerOrder(ctx, actor, lang, id)
	if failure != nil {
		return failure
	}
	fullReason := reason
	if notes != "" {
		if fullReason != "" {
			fullReason += " - " + notes
		} else {
			fullReason = notes
		}
	}
	if fullReason == "" {
		fullReason = i18n.T(lang, "orders.cancel_reason_buyer")
	}

	if err := h.commSvc.CancelOrder(ctx, id, &actor.UserID, fullReason); err != nil {
		return &orderCancelFailure{Back: fmt.Sprintf("/orders/%d", id),
			Message: fmt.Sprintf(i18n.T(lang, "orders.cancel_failed_with_err"), h.safeMessage(err, lang)), JSON: true}
	}

	if order.PaymentMethod == "wallet" && order.PaymentStatus == commerce.PaymentPaid {
		_ = h.refundOrderWalletPayment(ctx, order, nil, fullReason)
	}

	buyerName := h.resolveOrgName(ctx, actor.OrganizationID)
	orderNum := order.OrderNumber
	if orderNum == "" {
		orderNum = fmt.Sprintf("ORD-%d", order.ID)
	}
	for _, sh := range order.Shipments {
		if sh != nil && sh.OrganizationID > 0 {
			h.notifyOrderCancelledByBuyer(ctx, sh.OrganizationID, orderNum, buyerName, fullReason)
		}
	}
	return nil
}

// ReviewSubmit handles customer feedback submissions with multi-criteria rating.
