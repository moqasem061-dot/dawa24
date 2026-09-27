package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	"github.com/moqasem061-dot/dawa24/internal/modules/notifications"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// notifyOrderStatusChanged dispatches updates to the customer, vendor teams, and platform admin.
func (h *UIHandler) notifyOrderStatusChanged(
	ctx context.Context,
	order *commerce.Order,
	shipmentID int64,
	toStatus commerce.OrderStatus,
	vendorName string,
	notes string,
) {
	if order == nil {
		return
	}

	orderNum := order.OrderNumber
	if orderNum == "" {
		orderNum = fmt.Sprintf("ORD-%d", order.ID)
	}

	if vendorName == "" {
		vendorName = i18n.T("ar", "notif.the_vendor")
	}

	statusLabel := string(toStatus)
	switch toStatus {
	case commerce.StatusPending:
		statusLabel = "بانتظار التأكيد"
	case commerce.StatusProcessing:
		statusLabel = "قيد التجهيز"
	case commerce.StatusConfirmed:
		statusLabel = "تم التأكيد"
	case commerce.StatusShipped, commerce.StatusInTransit:
		statusLabel = "قيد الشحن والتوصيل"
	case commerce.StatusOutForDelivery:
		statusLabel = "مع المندوب للتسليم"
	case commerce.StatusDelivered, commerce.StatusCompleted:
		statusLabel = "تم التسليم بنجاح"
	case commerce.StatusCancelled:
		statusLabel = "ملغي"
	case commerce.StatusFailed:
		statusLabel = "فشل التوريد"
	case commerce.StatusReturned, commerce.StatusRefunded:
		statusLabel = "مرتجع / مسترد"
	}

	reasonStr := ""
	if strings.TrimSpace(notes) != "" {
		reasonStr = fmt.Sprintf("(ملاحظات: %s)", notes)
	}

	// 1. Notify Customer / Pharmacy
	vars := map[string]string{
		"order_number": orderNum,
		"vendor_name":  vendorName,
		"status":       statusLabel,
		"reason":       reasonStr,
	}

	h.dispatchEvent(ctx, notifications.EventOrderStatusChanged, order.CustomerID, order.OrganizationID, vars)
	if order.OrganizationID != nil && *order.OrganizationID > 0 {
		h.dispatchOrgBranchEvent(ctx, notifications.EventOrderStatusChanged, *order.OrganizationID, order.BranchID, vars)
	}

	// 2. Notify Vendor Organization(s)
	vendorTitle := fmt.Sprintf("تحديث حالة الطلب %s", orderNum)
	vendorBody := fmt.Sprintf("تم تحديث حالة الطلب إلى: %s %s", statusLabel, reasonStr)
	for _, sh := range order.Shipments {
		if sh == nil || sh.OrganizationID <= 0 {
			continue
		}
		if shipmentID > 0 && sh.ID != shipmentID {
			continue
		}
		h.dispatchOrgBranchNotification(ctx, sh.OrganizationID, sh.BranchID, "vendor.order.view", vendorTitle, vendorBody)
	}

	// 3. Notify Admin Team
	adminTitle := fmt.Sprintf("تحديث حالة طلب توريد %s", orderNum)
	buyerName := "صيدلية"
	if order.OrganizationID != nil {
		buyerName = h.resolveOrgName(ctx, *order.OrganizationID)
	}
	adminBody := fmt.Sprintf("تم تحديث حالة الطلب %s إلى: %s. المشتري: %s، المورد: %s %s", orderNum, statusLabel, buyerName, vendorName, reasonStr)
	h.dispatchAdminNotification(ctx, "commerce.order.view", adminTitle, adminBody)
}

// notifyOrderCancelledByBuyer dispatches notification to the supplier when a buyer cancels an order.
func (h *UIHandler) notifyOrderCancelledByBuyer(ctx context.Context, supplierOrgID int64, orderNum, customerName, reason string) {
	if supplierOrgID <= 0 {
		return
	}
	if customerName == "" {
		customerName = "المشتري"
	}
	if reason == "" {
		reason = "تم الإلغاء بواسطة العميل"
	}
	vars := map[string]string{
		"order_number":  orderNum,
		"customer_name": customerName,
		"reason":        reason,
	}
	h.dispatchOrgEvent(ctx, notifications.EventOrderCancelledByBuyer, supplierOrgID, vars)
}

// notifyPurchaseRequestResponded dispatches notification to the pharmacy when a vendor responds with prices.
func (h *UIHandler) notifyPurchaseRequestResponded(ctx context.Context, customerUserID int64, customerOrgID int64, vendorName string, requestID int64) {
	if vendorName == "" {
		vendorName = i18n.T("ar", "notif.the_vendor")
	}
	vars := map[string]string{
		"request_id":  fmt.Sprintf("%d", requestID),
		"vendor_name": vendorName,
	}
	var orgPtr *int64
	if customerOrgID > 0 {
		orgPtr = &customerOrgID
	}
	if customerUserID > 0 {
		h.dispatchEvent(ctx, notifications.EventPurchaseRequestResponded, customerUserID, orgPtr, vars)
	}
	if customerOrgID > 0 {
		h.dispatchOrgEvent(ctx, notifications.EventPurchaseRequestResponded, customerOrgID, vars)
	}
}

// notifyQuoteRequest dispatches notification to the vendor when a new RFQ is created.
func (h *UIHandler) notifyQuoteRequest(ctx context.Context, vendorOrgID int64, customerName string, productName string, quantity int) {
	if vendorOrgID <= 0 {
		return
	}
	if customerName == "" {
		customerName = i18n.T("ar", "notif.verified_pharmacy")
	}
	vars := map[string]string{
		"customer_name": customerName,
		"product_name":  productName,
		"quantity":      fmt.Sprintf("%d", quantity),
	}
	h.dispatchOrgEvent(ctx, notifications.EventQuoteRequested, vendorOrgID, vars)
}

// notifyQuoteProvided dispatches notification to the customer when a vendor submits a price quote.
func (h *UIHandler) notifyQuoteProvided(ctx context.Context, customerUserID int64, customerOrgID int64, vendorName string, productName string, quotePrice money.Amount) {
	if vendorName == "" {
		vendorName = i18n.T("ar", "notif.the_vendor")
	}
	vars := map[string]string{
		"vendor_name":  vendorName,
		"product_name": productName,
		"quote_price":  quotePrice.String(),
	}
	var orgPtr *int64
	if customerOrgID > 0 {
		orgPtr = &customerOrgID
	}
	if customerUserID > 0 {
		h.dispatchEvent(ctx, notifications.EventQuoteProvided, customerUserID, orgPtr, vars)
	}
	if customerOrgID > 0 {
		h.dispatchOrgEvent(ctx, notifications.EventQuoteProvided, customerOrgID, vars)
	}
}

// notifyQuoteDecision dispatches notification to the vendor when a customer accepts or rejects a price quote.
func (h *UIHandler) notifyQuoteDecision(ctx context.Context, vendorOrgID int64, customerName string, productName string, accepted bool) {
	if vendorOrgID <= 0 {
		return
	}
	if customerName == "" {
		customerName = i18n.T("ar", "notif.verified_pharmacy")
	}
	decision := "قبول"
	if !accepted {
		decision = "رفض"
	}
	vars := map[string]string{
		"customer_name": customerName,
		"product_name":  productName,
		"decision":      decision,
	}
	h.dispatchOrgEvent(ctx, notifications.EventQuoteDecision, vendorOrgID, vars)
}

// notifyNegotiationOffer dispatches notification when a customer proposes a negotiated price.
func (h *UIHandler) notifyNegotiationOffer(ctx context.Context, vendorOrgID int64, customerOrgName string, orderNum string, proposedAmount money.Amount) {
	if vendorOrgID <= 0 {
		return
	}
	if customerOrgName == "" {
		customerOrgName = i18n.T("ar", "notif.verified_pharmacy")
	}
	vars := map[string]string{
		"customer_name":   customerOrgName,
		"proposed_amount": proposedAmount.String(),
		"order_number":    orderNum,
	}
	h.dispatchOrgEvent(ctx, notifications.EventNegotiationOffer, vendorOrgID, vars)
}

// notifyNegotiationDecision dispatches notification to the pharmacy when a vendor accepts or rejects price negotiation.
func (h *UIHandler) notifyNegotiationDecision(ctx context.Context, customerUserID int64, customerOrgID int64, vendorName string, orderNum string, accepted bool, reason string) {
	if vendorName == "" {
		vendorName = i18n.T("ar", "notif.the_vendor")
	}
	decision := "قبول"
	if !accepted {
		decision = "رفض"
	}
	reasonStr := ""
	if strings.TrimSpace(reason) != "" {
		reasonStr = fmt.Sprintf("السبب: %s", reason)
	}
	vars := map[string]string{
		"order_number": orderNum,
		"vendor_name":  vendorName,
		"decision":     decision,
		"reason":       reasonStr,
	}
	var orgPtr *int64
	if customerOrgID > 0 {
		orgPtr = &customerOrgID
	}
	if customerUserID > 0 {
		h.dispatchEvent(ctx, notifications.EventNegotiationDecision, customerUserID, orgPtr, vars)
	}
	if customerOrgID > 0 {
		h.dispatchOrgEvent(ctx, notifications.EventNegotiationDecision, customerOrgID, vars)
	}
}

// notifySpecialOfferStatus dispatches notification when a special offer is approved or rejected.
func (h *UIHandler) notifySpecialOfferStatus(ctx context.Context, vendorOrgID int64, offerTitle string, approved bool, reason string) {
	if vendorOrgID <= 0 {
		return
	}
	status := "الموافقة على"
	if !approved {
		status = "رفض"
	}
	reasonStr := ""
	if strings.TrimSpace(reason) != "" {
		reasonStr = fmt.Sprintf("(السبب: %s)", reason)
	}
	vars := map[string]string{
		"status":      status,
		"offer_title": offerTitle,
		"reason":      reasonStr,
	}
	h.dispatchOrgEvent(ctx, notifications.EventSpecialOfferStatus, vendorOrgID, vars)
}

// notifyAdminsSpecialOfferSubmitted alerts platform staff that a supplier
// added a special offer that needs moderation.
func (h *UIHandler) notifyAdminsSpecialOfferSubmitted(ctx context.Context, vendorOrgID int64, offerTitle string) {
	h.dispatchAdminEvent(ctx, notifications.EventAdminSpecialOffer, map[string]string{
		"org_name":    h.resolveOrgName(ctx, vendorOrgID),
		"offer_title": offerTitle,
	})
}

// notifySponsorshipStatus dispatches notification when a sponsorship package request is approved or rejected.
func (h *UIHandler) notifySponsorshipStatus(ctx context.Context, vendorOrgID int64, pkgTitle string, approved bool, reason string) {
	if vendorOrgID <= 0 {
		return
	}
	status := "الموافقة على"
	if !approved {
		status = "رفض"
	}
	reasonStr := ""
	if strings.TrimSpace(reason) != "" {
		reasonStr = fmt.Sprintf("(السبب: %s)", reason)
	}
	vars := map[string]string{
		"status":        status,
		"package_title": pkgTitle,
		"reason":        reasonStr,
	}
	h.dispatchOrgEvent(ctx, notifications.EventSponsorshipStatus, vendorOrgID, vars)
}

// notifyAdStatus dispatches notification when an advertisement is approved or rejected.
func (h *UIHandler) notifyAdStatus(ctx context.Context, vendorOrgID int64, adTitle string, approved bool, reason string) {
	if vendorOrgID <= 0 {
		return
	}
	status := "الموافقة على"
	if !approved {
		status = "رفض"
	}
	reasonStr := ""
	if strings.TrimSpace(reason) != "" {
		reasonStr = fmt.Sprintf("(السبب: %s)", reason)
	}
	vars := map[string]string{
		"status":   status,
		"ad_title": adTitle,
		"reason":   reasonStr,
	}
	h.dispatchOrgEvent(ctx, notifications.EventAdStatus, vendorOrgID, vars)
}
