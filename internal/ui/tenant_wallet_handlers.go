package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func walletDestFor(actor authctx.Actor) string {
	if actor.IsVendor() {
		return "/vendor/wallet"
	}
	return "/customer/wallet"
}

// normalizeWalletStatusFilter keeps only the supported wallet status values:
// pending (قيد المراجعة), completed (مكتمل), rejected (مرفوض).
// "approved" is accepted as an alias of completed.
func normalizeWalletStatusFilter(v string) string {
	switch strings.TrimSpace(v) {
	case "pending":
		return "pending"
	case "completed", "approved":
		return "completed"
	case "rejected":
		return "rejected"
	default:
		return ""
	}
}

// normalizeWalletTypeFilter keeps only the supported wallet transaction types:
// deposit (إيداع), withdrawal (عملية سحب), purchase (مشتريات).
func normalizeWalletTypeFilter(v string) string {
	switch strings.TrimSpace(v) {
	case string(billing.TxDeposit), string(billing.TxWithdrawal), string(billing.TxPurchase):
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

// TenantWalletPage renders the dedicated unified Wallet, Balance, and Payment Methods page
// for Pharmacy customers and Vendors.
func (h *UIHandler) TenantWalletPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login?redirect="+r.URL.RequestURI(), http.StatusSeeOther)
		return
	}

	lang, dir := h.localeAndDir(r)
	isVendor := actor.IsVendor()

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	// Transaction status filter: pending (قيد المراجعة) | completed (مكتمل) | rejected (مرفوض).
	txStatus := normalizeWalletStatusFilter(r.URL.Query().Get("status"))
	// Transaction type filter: deposit (إيداع) | withdrawal (عملية سحب) | purchase (مشتريات).
	txType := normalizeWalletTypeFilter(r.URL.Query().Get("type"))

	var wallet *billing.Wallet
	var txs []*billing.WalletTransaction
	var totalTxCount int
	var depositRequests []*billing.WalletDeposit
	var paymentMethods []*billing.UserPaymentMethod
	var platformPaymentMethods []*billing.PlatformPaymentMethod

	walletUserID, candidateUIDs := resolveTenantUserIDs(ctx, h, actor)

	if h.billSvc != nil {
		seenPM := make(map[int64]bool)
		for _, uid := range candidateUIDs {
			if pms, err := h.billSvc.ListPaymentMethods(ctx, uid); err == nil {
				for _, pm := range pms {
					if pm != nil && !seenPM[pm.ID] {
						seenPM[pm.ID] = true
						paymentMethods = append(paymentMethods, pm)
					}
				}
			}
		}
		if ppms, err := h.billSvc.ListPlatformPaymentMethods(ctx, true); err == nil {
			platformPaymentMethods = ppms
		}
		// Deposit requests back the status filter (billing.wallet_deposits.status).
		// pending/rejected show only matching requests; completed shows the
		// ledger only (approved deposits already have ledger rows).
		// A type filter other than deposit hides deposit requests entirely.
		wantDeposits := txStatus != "completed" && (txType == "" || txType == string(billing.TxDeposit))
		if wantDeposits {
			depStatus := ""
			if txStatus == "pending" || txStatus == "rejected" {
				depStatus = txStatus
			}
			if deps, err := h.billSvc.ListUserDepositsWithStatus(ctx, walletUserID, depStatus, 50, 0); err == nil {
				depositRequests = deps
			}
		}

		// Withdrawal requests back the status filter (billing.wallet_withdrawals.status).
		// pending/rejected show matching requests; completed shows ledger only.
		// A type filter other than withdrawal hides withdrawal requests.
		var withdrawalRequests []*billing.WalletWithdrawal
		wantWithdrawals := (txType == "" || txType == string(billing.TxWithdrawal))
		if wantWithdrawals {
			withStatus := ""
			if txStatus == "pending" || txStatus == "rejected" {
				withStatus = txStatus
			}
			if withs, err := h.billSvc.ListUserWithdrawalsWithStatus(ctx, walletUserID, withStatus, 50, 0); err == nil {
				withdrawalRequests = withs
			}
		}

		if wItem, err := h.billSvc.GetWallet(ctx, walletUserID, "EGP"); err == nil && wItem != nil {
			wallet = wItem
			// Ledger rows back the type filter (billing.wallet_transactions.type).
			// A status of pending/rejected shows requests only, no ledger rows.
			if txStatus == "" || txStatus == "completed" {
				if list, total, err := h.billSvc.ListWalletTransactionsWithTypeTotal(ctx, wItem.ID, txType, limit, offset); err == nil {
					txs = list
					totalTxCount = total
				}
			}
		}

		viewData := pages.WalletViewData{
			IsVendor:               isVendor,
			Wallet:                 wallet,
			Transactions:           txs,
			DepositRequests:        depositRequests,
			WithdrawalRequests:     withdrawalRequests,
			PaymentMethods:         paymentMethods,
			PlatformPaymentMethods: platformPaymentMethods,
			NoticeType:             r.URL.Query().Get("notice"),
			NoticeMessage:          r.URL.Query().Get("msg"),
			TxStatus:               txStatus,
			TxType:                 txType,
			Page:                   page,
			PerPage:                limit,
			TotalCount:             totalTxCount,
		}

		h.renderPage(ctx, w, "render tenant wallet page", pages.WalletPage(viewData, lang, dir))
		return
	}

	viewData := pages.WalletViewData{
		IsVendor:      isVendor,
		NoticeType:    r.URL.Query().Get("notice"),
		NoticeMessage: r.URL.Query().Get("msg"),
		TxStatus:      txStatus,
		TxType:        txType,
		Page:          page,
		PerPage:       limit,
	}
	h.renderPage(ctx, w, "render tenant wallet page", pages.WalletPage(viewData, lang, dir))
}

// TenantWalletDepositSubmit handles wallet recharge deposit submissions.
func (h *UIHandler) TenantWalletDepositSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := walletDestFor(actor)

	_ = r.ParseMultipartForm(uploadMemoryBudget)
	amountStr := r.PostFormValue("amount")
	amt, err := money.Parse(amountStr)
	if err != nil || amt.IsZero() || amt.IsNegative() {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "wallet.deposit.invalid_amount"))
		return
	}

	platformMethodID := strings.TrimSpace(r.PostFormValue("platform_method_id"))
	senderAccount := strings.TrimSpace(r.PostFormValue("sender_account"))
	var senderPMIDPtr *int64
	if spmID, err := strconv.ParseInt(r.PostFormValue("sender_payment_method_id"), 10, 64); err == nil && spmID > 0 {
		senderPMIDPtr = &spmID
	}

	method := strings.TrimSpace(r.PostFormValue("payment_method"))
	if method == "" && platformMethodID != "" {
		method = platformMethodID
	}
	ref := strings.TrimSpace(r.PostFormValue("reference_number"))
	notes := strings.TrimSpace(r.PostFormValue("notes"))

	if method == "" {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "wallet.deposit.method_required"))
		return
	}
	if ref == "" {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "wallet.deposit.reference_required"))
		return
	}

	var attachmentURL string
	if file, _, err := r.FormFile("receipt"); err == nil && file != nil {
		_ = file.Close()
		if savedPath, err := saveUploadedFile(r, "receipt", "receipts"); err == nil {
			attachmentURL = savedPath
		}
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "wallet.service_unavailable"))
		return
	}

	walletUserID, candidateUIDs := resolveTenantUserIDs(ctx, h, actor)
	if senderPMIDPtr != nil && *senderPMIDPtr > 0 && senderAccount == "" {
		for _, uid := range candidateUIDs {
			if pm, err := h.billSvc.GetPaymentMethodByID(ctx, uid, *senderPMIDPtr); err == nil && pm != nil {
				senderAccount = pm.AccountIdentifier
				break
			}
		}
	}
	if senderAccount == "" && ref != "" {
		senderAccount = ref
	}

	var orgID int64 = actor.OrganizationID
	if orgID <= 0 && h.orgSvc != nil {
		if orgs, err := h.orgSvc.ListUserOrganizationsByUser(ctx, walletUserID); err == nil && len(orgs) > 0 {
			if orgs[0].CustomerOrgID != nil && *orgs[0].CustomerOrgID > 0 {
				orgID = *orgs[0].CustomerOrgID
			} else if orgs[0].VendorOrgID > 0 {
				orgID = orgs[0].VendorOrgID
			}
		}
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}

	if _, err := h.billSvc.RequestDepositExtended(ctx, walletUserID, orgPtr, "EGP", amt, method, ref, attachmentURL, notes, platformMethodID, senderAccount, senderPMIDPtr); err != nil {
		h.log.ErrorContext(ctx, "failed to submit deposit request", "error", err)
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, lang))
		return
	}

	// Dispatch in-app notification
	h.safeGo("notify-wallet-deposit-pending", func() {
		h.notifyWalletDeposit(context.Background(), walletUserID, orgID, amt, "pending")
	})

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "wallet.deposit.pending_success"))
}

// TenantWalletWithdrawSubmit handles wallet withdrawal requests.
func (h *UIHandler) TenantWalletWithdrawSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := walletDestFor(actor)

	_ = r.ParseForm()
	amountStr := r.PostFormValue("amount")
	amt, err := money.Parse(amountStr)
	if err != nil || amt.IsZero() || amt.IsNegative() {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "wallet.withdraw.invalid_amount"))
		return
	}

	payoutType := strings.TrimSpace(r.PostFormValue("payout_method_type"))
	destDetails := strings.TrimSpace(r.PostFormValue("destination_details"))
	destID := strings.TrimSpace(r.PostFormValue("destination_id"))
	if destDetails == "" && destID != "" {
		destDetails = destID
	}

	var userPMIDPtr *int64
	if pmID, err := strconv.ParseInt(r.PostFormValue("user_payment_method_id"), 10, 64); err == nil && pmID > 0 {
		userPMIDPtr = &pmID
	}

	walletUserID, candidateUIDs := resolveTenantUserIDs(ctx, h, actor)
	if userPMIDPtr != nil && *userPMIDPtr > 0 && (destDetails == "" || payoutType == "") {
		for _, uid := range candidateUIDs {
			if pm, err := h.billSvc.GetPaymentMethodByID(ctx, uid, *userPMIDPtr); err == nil && pm != nil {
				if destDetails == "" {
					destDetails = pm.AccountIdentifier
				}
				if payoutType == "" {
					payoutType = pm.Provider
				}
				break
			}
		}
	}

	if destDetails == "" {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "wallet.withdraw.destination_required"))
		return
	}
	if payoutType == "" {
		payoutType = "bank"
	}

	notes := strings.TrimSpace(r.PostFormValue("reason"))

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "wallet.service_unavailable"))
		return
	}

	var orgID int64 = actor.OrganizationID
	if orgID <= 0 && h.orgSvc != nil {
		if orgs, err := h.orgSvc.ListUserOrganizationsByUser(ctx, walletUserID); err == nil && len(orgs) > 0 {
			if orgs[0].CustomerOrgID != nil && *orgs[0].CustomerOrgID > 0 {
				orgID = *orgs[0].CustomerOrgID
			} else if orgs[0].VendorOrgID > 0 {
				orgID = orgs[0].VendorOrgID
			}
		}
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}

	// Verify wallet available balance
	wItem, err := h.billSvc.GetWallet(ctx, walletUserID, "EGP")
	if err != nil || wItem == nil || wItem.AvailableBalance.Minor() < amt.Minor() {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "wallet.withdraw.insufficient_funds"))
		return
	}

	if _, err := h.billSvc.RequestWithdrawal(ctx, walletUserID, orgPtr, "EGP", amt, payoutType, destDetails, userPMIDPtr, notes); err != nil {
		h.log.ErrorContext(ctx, "failed wallet withdrawal request", "error", err)
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, lang))
		return
	}

	// Dispatch in-app notification
	h.safeGo("notify-wallet-withdrawal-pending", func() {
		h.notifyWalletWithdrawal(context.Background(), walletUserID, orgID, amt, "pending")
	})

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "wallet.withdraw.pending_success"))
}
