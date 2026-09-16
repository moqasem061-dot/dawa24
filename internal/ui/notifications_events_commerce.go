package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// notifyOrderPlaced dispatches notifications to the customer and all fulfilling vendor teams.
func (h *UIHandler) notifyOrderPlaced(ctx context.Context, order *commerce.Order, pharmacyName string) {
	if order == nil {
		return
	}

	orderNum := order.OrderNumber
	if orderNum == "" {
		orderNum = fmt.Sprintf("ORD-%d", order.ID)
	}

	// 1. Notify Customer / Pharmacy
	custVars := map[string]string{
		"order_number": orderNum,
		"total_amount": order.TotalAmount.String(),
	}
	h.dispatchEvent(ctx, notifications.EventOrderPlaced, order.CustomerID, order.OrganizationID, custVars)
	if order.OrganizationID != nil && *order.OrganizationID > 0 {
		h.dispatchOrgBranchEvent(ctx, notifications.EventOrderPlaced, *order.OrganizationID, order.BranchID, custVars)
	}

	// 2. Notify each Vendor Organization
	if pharmacyName == "" {
		pharmacyName = i18n.T("ar", "notif.verified_pharmacy")
	}

	for _, sh := range order.Shipments {
		if sh == nil || sh.OrganizationID <= 0 {
			continue
		}
		itemCount := len(sh.Lines)
		vendorTitle := fmt.Sprintf(i18n.T("ar", "notif.new_supply_order_title"), orderNum)
		vendorBody := fmt.Sprintf(i18n.T("ar", "notif.new_supply_order_body"),
			pharmacyName, itemCount, sh.Subtotal.String())
		vendorBranch := sh.BranchID
		if vendorBranch == nil {
			vendorBranch = order.VendorBranchID
		}
		h.dispatchOrgBranchNotification(ctx, sh.OrganizationID, vendorBranch, "vendor.order.view", vendorTitle, vendorBody)
	}
}

// notifyOrderStatusChanged dispatches updates to the customer when a vendor updates shipment status.
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

	reasonStr := ""
	if strings.TrimSpace(notes) != "" {
		reasonStr = fmt.Sprintf("(ملاحظات: %s)", notes)
	}

	vars := map[string]string{
		"order_number": orderNum,
		"vendor_name":  vendorName,
		"status":       string(toStatus),
		"reason":       reasonStr,
	}

	h.dispatchEvent(ctx, notifications.EventOrderStatusChanged, order.CustomerID, order.OrganizationID, vars)
	if order.OrganizationID != nil && *order.OrganizationID > 0 {
		h.dispatchOrgBranchEvent(ctx, notifications.EventOrderStatusChanged, *order.OrganizationID, order.BranchID, vars)
	}
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

// notifyPurchaseRequestCreated dispatches notification to the vendor when a pharmacy submits a purchase request.
func (h *UIHandler) notifyPurchaseRequestCreated(ctx context.Context, vendorOrgID int64, pharmacyName string, requestID int64, itemCount int) {
	if vendorOrgID <= 0 {
		return
	}
	if pharmacyName == "" {
		pharmacyName = i18n.T("ar", "notif.a_pharmacy")
	}
	vars := map[string]string{
		"request_id":    fmt.Sprintf("%d", requestID),
		"pharmacy_name": pharmacyName,
		"item_count":    fmt.Sprintf("%d", itemCount),
	}
	h.dispatchOrgEvent(ctx, notifications.EventPurchaseRequestCreated, vendorOrgID, vars)
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

// notifySmartOrderRunFinished alerts the user when a smart-ordering calculation finishes.
func (h *UIHandler) notifySmartOrderRunFinished(ctx context.Context, userID int64, orgID int64, runID int64) {
	vars := map[string]string{
		"run_id": fmt.Sprintf("%d", runID),
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchEvent(ctx, notifications.EventSmartOrderRunFinished, userID, orgPtr, vars)
}

// notifySmartOrderRunFailed alerts the user when a smart-ordering run fails.
func (h *UIHandler) notifySmartOrderRunFailed(ctx context.Context, userID int64, orgID int64, runID int64, errMsg string) {
	vars := map[string]string{
		"run_id": fmt.Sprintf("%d", runID),
		"error":  errMsg,
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchEvent(ctx, notifications.EventSmartOrderRunFailed, userID, orgPtr, vars)
}

// notifyImportRunFinished alerts the user when an import finishes committing.
func (h *UIHandler) notifyImportRunFinished(ctx context.Context, userID int64, orgID int64, runID int64, rowCount int) {
	vars := map[string]string{
		"run_id":     fmt.Sprintf("%d", runID),
		"rows_count": fmt.Sprintf("%d", rowCount),
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchEvent(ctx, notifications.EventImportRunFinished, userID, orgPtr, vars)
}

// notifyImportRunFailed alerts the user when an import run fails.
func (h *UIHandler) notifyImportRunFailed(ctx context.Context, userID int64, orgID int64, runID int64, errMsg string) {
	vars := map[string]string{
		"run_id": fmt.Sprintf("%d", runID),
		"error":  errMsg,
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchEvent(ctx, notifications.EventImportRunFailed, userID, orgPtr, vars)
}

// notifyQuotaExhausted alerts the buying organization when a branch quota is reached.
func (h *UIHandler) notifyQuotaExhausted(ctx context.Context, buyingOrgID int64, branchName, vendorName, productName string) {
	if buyingOrgID <= 0 {
		return
	}
	vars := map[string]string{
		"branch_name":  branchName,
		"vendor_name":  vendorName,
		"product_name": productName,
	}
	h.dispatchOrgEvent(ctx, notifications.EventQuotaExhausted, buyingOrgID, vars)
}

// notifyQuotaReleased alerts the buying organization when a supplier releases quota.
func (h *UIHandler) notifyQuotaReleased(ctx context.Context, buyingOrgID int64, branchName, vendorName, productName string) {
	if buyingOrgID <= 0 {
		return
	}
	vars := map[string]string{
		"branch_name":  branchName,
		"vendor_name":  vendorName,
		"product_name": productName,
	}
	h.dispatchOrgEvent(ctx, notifications.EventQuotaReleased, buyingOrgID, vars)
}
