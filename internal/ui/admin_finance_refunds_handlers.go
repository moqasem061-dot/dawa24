package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// AdminTransactionRefundSubmit processes a refund request for a specific wallet transaction.
func (h *UIHandler) AdminTransactionRefundSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/finance/transactions?tab=transactions", http.StatusSeeOther)
		return
	}

	txID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || txID <= 0 {
		h.redirectWithNotice(w, r, "/admin/finance/transactions?tab=transactions", "error", i18n.T(lang, "admin.finance.invalid_transaction_id"))
		return
	}

	_ = r.ParseForm()
	reason := strings.TrimSpace(r.PostFormValue("reason"))
	if reason == "" {
		h.redirectWithNotice(w, r, "/admin/finance/transactions?tab=transactions", "error", i18n.T(lang, "admin.finance.refund_reason_required"))
		return
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/finance/transactions?tab=transactions", "error", i18n.T(lang, "admin.finance.service_unavailable"))
		return
	}

	refundTx, err := h.billSvc.AdminRefundTransaction(ctx, txID, reason, actor.UserID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to refund transaction", "error", err, "transaction_id", txID)
		h.redirectWithNotice(w, r, "/admin/finance/transactions?tab=transactions", "error", h.safeMessage(err, lang))
		return
	}

	// Dispatch notification asynchronously
	if refundTx != nil {
		go func() {
			bgCtx := context.Background()
			wallet, err := h.billSvc.GetWalletByID(bgCtx, refundTx.WalletID)
			if err == nil && wallet != nil {
				var orgID int64
				if wallet.OrganizationID != nil {
					orgID = *wallet.OrganizationID
				}
				h.notifyWalletTransactionRefund(bgCtx, wallet.UserID, orgID, txID, refundTx.Amount, reason)
			}
		}()
	}

	h.redirectWithNotice(w, r, "/admin/finance/transactions?tab=transactions", "success", i18n.T(lang, "admin.finance.refund_success"))
}

// AdminDepositRefundSubmit processes a refund request for an approved deposit.
func (h *UIHandler) AdminDepositRefundSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/finance/deposits?tab=deposits", http.StatusSeeOther)
		return
	}

	depositID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || depositID <= 0 {
		h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "error", i18n.T(lang, "admin.finance.invalid_deposit_id"))
		return
	}

	_ = r.ParseForm()
	reason := strings.TrimSpace(r.PostFormValue("reason"))
	if reason == "" {
		h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "error", i18n.T(lang, "admin.finance.refund_reason_required"))
		return
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "error", i18n.T(lang, "admin.finance.service_unavailable"))
		return
	}

	dep, refundTx, err := h.billSvc.AdminRefundDeposit(ctx, depositID, reason, actor.UserID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to refund deposit", "error", err, "deposit_id", depositID)
		h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "error", h.safeMessage(err, lang))
		return
	}

	// Dispatch notification asynchronously
	if dep != nil && refundTx != nil {
		var orgID int64
		if dep.OrganizationID != nil {
			orgID = *dep.OrganizationID
		}
		var origTxID int64
		if dep.TransactionID != nil {
			origTxID = *dep.TransactionID
		}
		h.safeGo("notify-wallet-transaction-refund", func() {
			h.notifyWalletTransactionRefund(context.Background(), dep.UserID, orgID, origTxID, refundTx.Amount, reason)
		})
	}

	h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "success", i18n.T(lang, "admin.finance.refund_success"))
}
