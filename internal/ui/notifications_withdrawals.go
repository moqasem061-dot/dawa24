package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// notifyWalletDeposit dispatches notification when a deposit request is submitted or credited.
func (h *UIHandler) notifyWalletDeposit(ctx context.Context, userID int64, orgID int64, amount money.Amount, status string) {
	var title, body string
	if status == "approved" || status == "completed" {
		title = i18n.T("ar", "notif.wallet_deposit_approved_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.wallet_deposit_approved_body"), amount.String())
	} else {
		title = i18n.T("ar", "notif.wallet_deposit_pending_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.wallet_deposit_pending_body"), amount.String())
	}
	perm := "vendor.wallet.view"
	if orgID > 0 && h.orgSvc != nil {
		if orgObj, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), orgID); err == nil && orgObj != nil {
			if orgObj.Type == "pharmacy" || orgObj.Type == "customer" {
				perm = "pharmacy.wallet.view"
			}
		}
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchInAppNotification(ctx, userID, orgPtr, perm, title, body)
	if orgID > 0 {
		h.dispatchOrgNotification(ctx, orgID, perm, title, body)
	}
	if status == "pending" {
		orgName := ""
		if orgID > 0 && h.orgSvc != nil {
			if orgObj, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), orgID); err == nil && orgObj != nil {
				orgName = orgObj.Name.Get("ar")
				if orgName == "" {
					orgName = orgObj.Name.Get("en")
				}
			}
		}
		adminTitle := "طلب شحن محفظة جديد بحاجة للمراجعة"
		var adminBody string
		if orgName != "" {
			adminBody = fmt.Sprintf("قدمت منشأة (%s) طلب شحن محفظة جديد بمبلغ %s ج.م، يرجى المراجعة والاعتماد.", orgName, amount.String())
		} else {
			adminBody = fmt.Sprintf("ورد طلب شحن محفظة جديد بمبلغ %s ج.م، يرجى المراجعة والاعتماد.", amount.String())
		}
		h.dispatchAdminNotification(ctx, "billing.payment.view", adminTitle, adminBody)
	}
}

// notifyWalletDepositRejected dispatches notification when a deposit request is rejected with reason.
func (h *UIHandler) notifyWalletDepositRejected(ctx context.Context, userID int64, orgID int64, amount money.Amount, reason string) {
	title := i18n.T("ar", "notif.wallet_deposit_rejected_title")
	body := fmt.Sprintf(i18n.T("ar", "notif.wallet_deposit_rejected_body"), amount.String())
	if strings.TrimSpace(reason) != "" {
		body += fmt.Sprintf(i18n.T("ar", "notif.reason_prefix"), reason)
	}
	perm := "vendor.wallet.view"
	if orgID > 0 && h.orgSvc != nil {
		if orgObj, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), orgID); err == nil && orgObj != nil {
			if orgObj.Type == "pharmacy" || orgObj.Type == "customer" {
				perm = "pharmacy.wallet.view"
			}
		}
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchInAppNotification(ctx, userID, orgPtr, perm, title, body)
	if orgID > 0 {
		h.dispatchOrgNotification(ctx, orgID, perm, title, body)
	}
}

// notifyWalletWithdrawal dispatches notification when a withdrawal request is submitted or approved.
func (h *UIHandler) notifyWalletWithdrawal(ctx context.Context, userID int64, orgID int64, amount money.Amount, status string) {
	var title, body string
	if status == "approved" || status == "completed" {
		title = i18n.T("ar", "notif.wallet_withdrawal_approved_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.wallet_withdrawal_approved_body"), amount.String())
	} else {
		title = i18n.T("ar", "notif.wallet_withdrawal_pending_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.wallet_withdrawal_pending_body"), amount.String())
	}
	perm := "vendor.wallet.view"
	if orgID > 0 && h.orgSvc != nil {
		if orgObj, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), orgID); err == nil && orgObj != nil {
			if orgObj.Type == "pharmacy" || orgObj.Type == "customer" {
				perm = "pharmacy.wallet.view"
			}
		}
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchInAppNotification(ctx, userID, orgPtr, perm, title, body)
	if orgID > 0 {
		h.dispatchOrgNotification(ctx, orgID, perm, title, body)
	}
	if status == "pending" {
		orgName := ""
		if orgID > 0 && h.orgSvc != nil {
			if orgObj, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), orgID); err == nil && orgObj != nil {
				orgName = orgObj.Name.Get("ar")
				if orgName == "" {
					orgName = orgObj.Name.Get("en")
				}
			}
		}
		adminTitle := "طلب سحب رصيد جديد بحاجة للمراجعة"
		var adminBody string
		if orgName != "" {
			adminBody = fmt.Sprintf("قدمت منشأة (%s) طلب سحب رصيد بمبلغ %s ج.م، يرجى المراجعة والتحويل والاعتماد.", orgName, amount.String())
		} else {
			adminBody = fmt.Sprintf("ورد طلب سحب رصيد جديد بمبلغ %s ج.م، يرجى المراجعة والتحويل والاعتماد.", amount.String())
		}
		h.dispatchAdminNotification(ctx, "billing.payment.view", adminTitle, adminBody)
	}
}

// notifyWalletWithdrawalRejected dispatches notification when a withdrawal request is rejected with reason.
func (h *UIHandler) notifyWalletWithdrawalRejected(ctx context.Context, userID int64, orgID int64, amount money.Amount, reason string) {
	title := i18n.T("ar", "notif.wallet_withdrawal_rejected_title")
	body := fmt.Sprintf(i18n.T("ar", "notif.wallet_withdrawal_rejected_body"), amount.String())
	if strings.TrimSpace(reason) != "" {
		body += fmt.Sprintf(i18n.T("ar", "notif.reason_prefix"), reason)
	}
	perm := "vendor.wallet.view"
	if orgID > 0 && h.orgSvc != nil {
		if orgObj, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), orgID); err == nil && orgObj != nil {
			if orgObj.Type == "pharmacy" || orgObj.Type == "customer" {
				perm = "pharmacy.wallet.view"
			}
		}
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchInAppNotification(ctx, userID, orgPtr, perm, title, body)
	if orgID > 0 {
		h.dispatchOrgNotification(ctx, orgID, perm, title, body)
	}
}

// notifyRefundIssued alerts an organization when a refund is issued to its wallet.
func (h *UIHandler) notifyRefundIssued(ctx context.Context, orgID int64, orderNum string, amount money.Amount, reason string) {
	if orgID <= 0 {
		return
	}
	if reason == "" {
		reason = "استرداد للطلب"
	}
	vars := map[string]string{
		"order_number": orderNum,
		"amount":       amount.String(),
		"reason":       reason,
	}
	h.dispatchOrgEvent(ctx, notifications.EventRefundIssued, orgID, vars)
}

// notifyWalletTransactionRefund alerts an organization when an admin refunds a wallet transaction or deposit.
func (h *UIHandler) notifyWalletTransactionRefund(ctx context.Context, userID int64, orgID int64, origTxID int64, amount money.Amount, reason string) {
	title := "تم استرداد مبلغ إلى محفظتك"
	body := fmt.Sprintf("تم استرداد مبلغ %s ج.م إلى محفظة المنشأة مقابل المعاملة #TX-%d", amount.String(), origTxID)
	if strings.TrimSpace(reason) != "" {
		body += fmt.Sprintf(" (السبب: %s)", reason)
	}
	perm := "vendor.wallet.view"
	if orgID > 0 && h.orgSvc != nil {
		if orgObj, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), orgID); err == nil && orgObj != nil {
			if orgObj.Type == "pharmacy" || orgObj.Type == "customer" {
				perm = "pharmacy.wallet.view"
			}
		}
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchInAppNotification(ctx, userID, orgPtr, perm, title, body)
	if orgID > 0 {
		h.dispatchOrgNotification(ctx, orgID, perm, title, body)
		vars := map[string]string{
			"order_number": fmt.Sprintf("#TX-%d", origTxID),
			"amount":       amount.String(),
			"reason":       reason,
		}
		h.dispatchOrgEvent(ctx, notifications.EventRefundIssued, orgID, vars)
	}
}
