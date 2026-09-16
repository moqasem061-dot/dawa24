package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// AdminWithdrawalApproveSubmit approves a pending withdrawal request and debits the user's wallet.
func (h *UIHandler) AdminWithdrawalApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/finance/withdrawals?tab=withdrawals", http.StatusSeeOther)
		return
	}

	withdrawalID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || withdrawalID <= 0 {
		h.redirectWithNotice(w, r, "/admin/finance/withdrawals?tab=withdrawals", "error", i18n.T(lang, "admin.finance.invalid_withdrawal_id"))
		return
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/finance/withdrawals?tab=withdrawals", "error", i18n.T(lang, "admin.finance.service_unavailable"))
		return
	}

	_ = r.ParseMultipartForm(10 << 20)
	var transferReceiptURL string
	if file, _, err := r.FormFile("transfer_receipt"); err == nil && file != nil {
		_ = file.Close()
		if savedPath, err := saveUploadedFile(r, "transfer_receipt", "receipts"); err == nil {
			transferReceiptURL = savedPath
		}
	}
	if transferReceiptURL == "" {
		transferReceiptURL = strings.TrimSpace(r.PostFormValue("receipt_url"))
	}

	withdrawal, tx, err := h.billSvc.AdminApproveWithdrawal(ctx, withdrawalID, actor.UserID, transferReceiptURL)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to approve withdrawal", "error", err, "withdrawal_id", withdrawalID)
		h.redirectWithNotice(w, r, "/admin/finance/withdrawals?tab=withdrawals", "error", h.safeMessage(err, lang))
		return
	}

	if withdrawal != nil {
		var orgID int64
		if withdrawal.OrganizationID != nil {
			orgID = *withdrawal.OrganizationID
		}
		h.safeGo("notify-wallet-withdrawal-approved", func() {
			h.notifyWalletWithdrawal(context.Background(), withdrawal.UserID, orgID, withdrawal.Amount, "approved")
		})
	}

	h.redirectWithNotice(w, r, "/admin/finance/withdrawals?tab=withdrawals", "success", fmt.Sprintf(i18n.T(lang, "admin.finance.withdrawal_approved_success_format"), withdrawal.Amount.String(), tx.ID))
}

// AdminWithdrawalRejectSubmit rejects a pending withdrawal request with an explicit reason.
func (h *UIHandler) AdminWithdrawalRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/finance/withdrawals?tab=withdrawals", http.StatusSeeOther)
		return
	}

	withdrawalID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || withdrawalID <= 0 {
		h.redirectWithNotice(w, r, "/admin/finance/withdrawals?tab=withdrawals", "error", i18n.T(lang, "admin.finance.invalid_withdrawal_id"))
		return
	}

	_ = r.ParseForm()
	reason := strings.TrimSpace(r.PostFormValue("rejection_reason"))
	if reason == "" {
		reason = i18n.T(lang, "admin.finance.default_withdrawal_rejection_reason")
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/finance/withdrawals?tab=withdrawals", "error", i18n.T(lang, "admin.finance.service_unavailable"))
		return
	}

	withdrawal, err := h.billSvc.AdminRejectWithdrawal(ctx, withdrawalID, actor.UserID, reason)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to reject withdrawal", "error", err, "withdrawal_id", withdrawalID)
		h.redirectWithNotice(w, r, "/admin/finance/withdrawals?tab=withdrawals", "error", h.safeMessage(err, lang))
		return
	}

	if withdrawal != nil {
		var orgID int64
		if withdrawal.OrganizationID != nil {
			orgID = *withdrawal.OrganizationID
		}
		h.safeGo("notify-wallet-withdrawal-rejected", func() {
			h.notifyWalletWithdrawalRejected(context.Background(), withdrawal.UserID, orgID, withdrawal.Amount, reason)
		})
	}

	h.redirectWithNotice(w, r, "/admin/finance/withdrawals?tab=withdrawals", "success", i18n.T(lang, "admin.finance.withdrawal_rejected_success"))
}
