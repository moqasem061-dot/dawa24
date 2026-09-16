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
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// WalletPage redirects to the dedicated wallet page.
func (h *UIHandler) WalletPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login?redirect=/wallet", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, walletDestFor(actor), http.StatusSeeOther)
}

// WalletDepositSubmit handles submitting a funds deposit request, placing it in pending status for admin review.
func (h *UIHandler) WalletDepositSubmit(w http.ResponseWriter, r *http.Request) {
	h.TenantWalletDepositSubmit(w, r)
}

// WalletDepositEditSubmit handles updating a pending deposit request before admin review.
func (h *UIHandler) WalletDepositEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := walletDestFor(actor)

	depositID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || depositID <= 0 {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "wallet.deposit.invalid_id"))
		return
	}

	_ = r.ParseMultipartForm(uploadMemoryBudget)
	amountStr := r.PostFormValue("amount")
	amt, err := money.Parse(amountStr)
	if err != nil || amt.IsZero() || amt.IsNegative() {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "wallet.deposit.invalid_amount"))
		return
	}

	method := strings.TrimSpace(r.PostFormValue("payment_method"))
	ref := strings.TrimSpace(r.PostFormValue("reference_number"))
	notes := strings.TrimSpace(r.PostFormValue("notes"))

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

	if _, err := h.billSvc.EditPendingDeposit(ctx, actor.UserID, depositID, amt, method, ref, attachmentURL, notes); err != nil {
		h.log.ErrorContext(ctx, "failed to update pending deposit", "error", err, "deposit_id", depositID)
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "wallet.deposit.updated_success"))
}

// WalletWithdrawSubmit handles submitting a funds withdrawal request and debiting the wallet.
func (h *UIHandler) WalletWithdrawSubmit(w http.ResponseWriter, r *http.Request) {
	h.TenantWalletWithdrawSubmit(w, r)
}

// TenantSubscriptionCheckoutSubmit handles purchasing/upgrading a plan with wallet deduction.
func (h *UIHandler) TenantSubscriptionCheckoutSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || (actor.OrganizationID <= 0 && actor.UserID <= 0) {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	redirectURL := "/customer/subscription"
	if actor.IsVendor() {
		redirectURL = "/vendor/subscription"
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "wallet.invalid_data"))
		return
	}

	planSlug := strings.TrimSpace(r.FormValue("plan_slug"))
	billingCycle := strings.TrimSpace(r.FormValue("billing_cycle"))
	autoRenew := r.FormValue("auto_renew") == "1" || r.FormValue("auto_renew") == "true" || r.FormValue("auto_renew") == "on"

	if planSlug == "" {
		h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "subscription.plan_required"))
		return
	}

	if billingCycle != "annual" && billingCycle != "monthly" {
		billingCycle = "monthly"
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, redirectURL, "error", i18n.T(lang, "subscription.service_unavailable"))
		return
	}

	walletUserID, _ := resolveTenantUserIDs(ctx, h, actor)

	// Check if this action represents an upgrade from an active plan
	var isUpgrade bool
	targetPlan, planErr := h.billSvc.GetPlanBySlug(ctx, planSlug)
	if planErr == nil && targetPlan != nil {
		if actor.OrganizationID > 0 {
			if curSub, err := h.billSvc.GetActiveSubscriptionByOrg(ctx, actor.OrganizationID); err == nil && curSub != nil {
				if curSub.PlanID != targetPlan.ID {
					isUpgrade = true
				}
			}
		} else {
			if curSub, err := h.billSvc.GetActiveSubscription(ctx, walletUserID); err == nil && curSub != nil {
				if curSub.PlanID != targetPlan.ID {
					isUpgrade = true
				}
			}
		}
	}

	_, err := h.billSvc.SubscribeWithWallet(ctx, walletUserID, orgPtr, planSlug, billingCycle, autoRenew)
	if err != nil {
		h.log.ErrorContext(ctx, "subscription checkout failed", "error", err, "user_id", walletUserID, "plan", planSlug)
		h.redirectWithNotice(w, r, redirectURL, "error", h.safeMessage(err, lang))
		return
	}

	planDisplayName := planSlug
	var planCost money.Amount
	if targetPlan != nil {
		planDisplayName = targetPlan.Name.Get("ar")
		if planDisplayName == "" {
			planDisplayName = targetPlan.Name.Get("en")
		}
		if planDisplayName == "" {
			planDisplayName = targetPlan.Slug
		}
		if billingCycle == "annual" {
			planCost = targetPlan.PriceYear
		} else {
			planCost = targetPlan.PriceMonth
		}
	}

	// Dispatch notification to user & org
	h.safeGo("notify-subscription-updated", func() {
		h.notifySubscriptionUpdated(context.Background(), walletUserID, actor.OrganizationID, planDisplayName, billingCycle, planCost, isUpgrade)
	})

	var successMsg string
	if isUpgrade {
		successMsg = fmt.Sprintf("تمت ترقية باقتك بنجاح إلى باقة %s! تم تصفير الاستهلاك القديم وبدء دورة استهلاك جديدة بكامل مميزات وحصة الباقة الجديدة.", planDisplayName)
	} else {
		successMsg = fmt.Sprintf("تم تفعيل اشتراكك بنجاح في باقة %s وبدء دورة الاستهلاك الجديدة بكامل مميزاتها.", planDisplayName)
	}

	h.redirectWithNotice(w, r, redirectURL, "success", successMsg)
}
