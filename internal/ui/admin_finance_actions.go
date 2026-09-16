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
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminRecordInvoicePaymentSubmit records an invoice payment directly from the admin invoices page.
func (h *UIHandler) AdminRecordInvoicePaymentSubmit(w http.ResponseWriter, r *http.Request) {
	h.VendorRecordPaymentSubmit(w, r)
}

// AdminDepositApproveSubmit approves a pending deposit request and credits the user's wallet.
func (h *UIHandler) AdminDepositApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/finance?tab=deposits", http.StatusSeeOther)
		return
	}

	depositID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || depositID <= 0 {
		h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "error", i18n.T(lang, "admin.finance.invalid_deposit_id"))
		return
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "error", i18n.T(lang, "admin.finance.service_unavailable"))
		return
	}

	dep, tx, err := h.billSvc.AdminApproveDeposit(ctx, depositID, actor.UserID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to approve deposit", "error", err, "deposit_id", depositID)
		h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "error", h.safeMessage(err, lang))
		return
	}

	if dep != nil {
		var orgID int64
		if dep.OrganizationID != nil {
			orgID = *dep.OrganizationID
		}
		h.safeGo("notify-wallet-deposit-approved", func() {
			h.notifyWalletDeposit(context.Background(), dep.UserID, orgID, dep.Amount, "approved")
		})
	}

	h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "success", fmt.Sprintf(i18n.T(lang, "admin.finance.deposit_approved_success_format"), dep.Amount.String(), tx.ID))
}

// AdminDepositRejectSubmit rejects a pending deposit request with an explicit reason.
func (h *UIHandler) AdminDepositRejectSubmit(w http.ResponseWriter, r *http.Request) {
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
	reason := strings.TrimSpace(r.PostFormValue("rejection_reason"))
	if reason == "" {
		reason = i18n.T(lang, "admin.finance.default_deposit_rejection_reason")
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "error", i18n.T(lang, "admin.finance.service_unavailable"))
		return
	}

	dep, err := h.billSvc.AdminRejectDeposit(ctx, depositID, actor.UserID, reason)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to reject deposit", "error", err, "deposit_id", depositID)
		h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "error", h.safeMessage(err, lang))
		return
	}

	if dep != nil {
		var orgID int64
		if dep.OrganizationID != nil {
			orgID = *dep.OrganizationID
		}
		h.safeGo("notify-wallet-deposit-rejected", func() {
			h.notifyWalletDepositRejected(context.Background(), dep.UserID, orgID, dep.Amount, reason)
		})
	}

	h.redirectWithNotice(w, r, "/admin/finance/deposits?tab=deposits", "success", i18n.T(lang, "admin.finance.deposit_rejected_success"))
}

// AdminWalletAdjustSubmit handles manual balance adjustment/credit/debit for a wallet.
func (h *UIHandler) AdminWalletAdjustSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor := authctx.FromContext(ctx)
	actorID := actor.UserID

	walletID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || walletID <= 0 {
		h.redirectWithNotice(w, r, "/admin/finance/wallets?tab=wallets", "error", i18n.T(lang, "admin.finance.invalid_wallet_id"))
		return
	}

	actionType := strings.TrimSpace(r.FormValue("action_type")) // "deposit", "withdrawal", "adjustment"
	amountStr := strings.TrimSpace(r.FormValue("amount"))
	reason := strings.TrimSpace(r.FormValue("reason"))

	amt, parseErr := money.Parse(amountStr)
	if parseErr != nil || amt.IsZero() || amt.IsNegative() {
		h.redirectWithNotice(w, r, "/admin/finance/wallets?tab=wallets", "error", i18n.T(lang, "admin.finance.invalid_amount"))
		return
	}
	if reason == "" {
		reason = i18n.T(lang, "admin.finance.default_adjustment_reason")
	}

	var txType billing.TransactionType

	switch actionType {
	case "deposit":
		txType = billing.TxDeposit
	case "withdrawal":
		txType = billing.TxWithdrawal
		amt = money.FromMinor(-amt.Minor())
	default:
		txType = billing.TxAdjustment
		if r.FormValue("is_deduct") == "true" {
			amt = money.FromMinor(-amt.Minor())
		}
	}

	if h.billSvc != nil {
		if err := h.billSvc.AdminPerformWalletAdjustment(ctx, walletID, amt, txType, reason, actorID); err != nil {
			h.redirectWithNotice(w, r, "/admin/finance/wallets?tab=wallets", "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/admin/finance/transactions?tab=transactions&wallet_id=%d", walletID), "success", i18n.T(lang, "admin.finance.wallet_adjusted_success"))
}

// AdminOfferOrderDetailPage renders single offer order details.
func (h *UIHandler) AdminOfferOrderDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	orderID, _ := strconv.ParseInt(idStr, 10, 64)

	var order *commerce.Order
	if h.commSvc != nil && orderID > 0 {
		order, _ = h.commSvc.GetOrder(database.AsSystem(ctx), orderID)
	}

	if order == nil {
		http.Redirect(w, r, "/admin/orders?tab=offers", http.StatusSeeOther)
		return
	}

	h.renderPage(ctx, w, "render admin offer order detail", pages.AdminOfferOrderDetailPage(order, lang, dir))
}
