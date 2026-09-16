package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// SettingsIndex renders the comprehensive unified tab-based account settings hub.
func (h *UIHandler) SettingsIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings", http.StatusSeeOther)
		return
	}
	// A supplier used to be redirected from here to /vendor/organization.
	//
	// Those are two different things: this page is the caller's own account —
	// their name, their password, their devices — and that one is the company's
	// commercial profile. The redirect meant a supplier had no way to reach
	// their own account settings at all, and landed instead on a page that
	// answers 404 without vendor.organization.view.

	var user *identity.User
	var sessions []*identity.Session
	var sessionPlans []*identity.SessionPlan
	var paymentMethods []*billing.UserPaymentMethod
	var wallet *billing.Wallet
	var txs []*billing.WalletTransaction

	var platformPaymentMethods []*billing.PlatformPaymentMethod

	if h.idSvc != nil {
		if me, err := h.idSvc.GetMe(ctx, actor.UserID, nil); err != nil {
			h.log.DebugContext(ctx, "settings: get me optional", "error", err)
		} else if me != nil {
			user = me.User
		}
		if sess, err := h.idSvc.ListSessions(ctx, actor.UserID); err != nil {
			h.log.WarnContext(ctx, "settings: list sessions", "error", err)
		} else {
			sessions = sess
		}
		if plans, err := h.idSvc.ListSessionPlans(ctx); err != nil {
			h.log.WarnContext(ctx, "settings: list session plans", "error", err)
		} else {
			sessionPlans = plans
		}
	}

	var depositRequests []*billing.WalletDeposit

	if h.billSvc != nil {
		if pms, err := h.billSvc.ListPaymentMethods(ctx, actor.UserID); err != nil {
			h.log.WarnContext(ctx, "settings: list payment methods", "error", err)
		} else {
			paymentMethods = pms
		}
		if ppms, err := h.billSvc.ListPlatformPaymentMethods(ctx, true); err != nil {
			h.log.WarnContext(ctx, "settings: list platform payment methods", "error", err)
		} else {
			platformPaymentMethods = ppms
		}
		if deps, err := h.billSvc.ListUserDeposits(ctx, actor.UserID, 50, 0); err != nil {
			h.log.WarnContext(ctx, "settings: list user deposits", "error", err)
		} else {
			depositRequests = deps
		}
		if w, err := h.billSvc.GetWallet(ctx, actor.UserID, "EGP"); err != nil {
			h.log.DebugContext(ctx, "settings: get wallet optional", "error", err)
		} else if w != nil {
			wallet = w
			if list, err := h.billSvc.ListWalletTransactions(ctx, w.ID, 50, 0); err != nil {
				h.log.WarnContext(ctx, "settings: list wallet transactions", "error", err)
			} else {
				txs = list
			}
		}
	}

	if user == nil {
		user = &identity.User{
			ID:    actor.UserID,
			Email: "user@dawa24.eg",
			Name:  i18n.Text{i18n.AR: i18n.TDefault("w4_ui.s_98_98"), i18n.EN: "Verified Pharmacist"},
			Role:  actor.Role,
		}
	}

	isEmployee := (actor.OrganizationID > 0 && !actor.IsOwner) || actor.Role == "employee" || actor.Role == "org_employee"
	isOwner := actor.IsOwner || (actor.OrganizationID > 0 && !isEmployee)
	var pendingAccountDel *identity.AccountDeletionRequest
	if h.idSvc != nil {
		pendingAccountDel, _ = h.idSvc.GetPendingAccountDeletion(ctx, actor.UserID)
	}

	var pendingOrgDel *org.OrganizationDeletionRequest
	orgName := ""
	orgLegalName := ""
	if actor.OrganizationID > 0 && isOwner && h.orgSvc != nil {
		pendingOrgDel, _ = h.orgSvc.GetPendingOrganizationDeletion(ctx, actor.OrganizationID)
		if o, err := h.orgSvc.GetOrganization(ctx, actor.OrganizationID); err == nil && o != nil {
			if display := o.TradeName.Get(i18n.ParseLang(lang)); display != "" {
				orgName = display
			} else if o.LegalName != "" {
				orgName = o.LegalName
			}
			if orgLegalName == "" {
				orgLegalName = o.LegalName
			}
			if orgLegalName == "" {
				orgLegalName = orgName
			}
		}
	}

	var prefs *identity.UserPreferences
	if h.idSvc != nil {
		prefs, _ = h.idSvc.GetPreferences(ctx, actor.UserID)
	}

	data := pages.UnifiedSettingsData{
		User:                   user,
		Wallet:                 wallet,
		Transactions:           txs,
		DepositRequests:        depositRequests,
		PaymentMethods:         paymentMethods,
		PlatformPaymentMethods: platformPaymentMethods,
		Sessions:               sessions,
		SessionPlans:           sessionPlans,
		ActiveTab:              "profile",
		CanRequestDeletion:     !isEmployee,
		IsOwner:                isOwner,
		OrgID:                  actor.OrganizationID,
		OrgName:                orgName,
		OrgLegalName:           orgLegalName,
		PendingAccountDeletion: pendingAccountDel,
		PendingOrgDeletion:     pendingOrgDel,
		Preferences:            prefs,
		TelegramEnabled:        h.telegramEnabled(),
		WhatsAppEnabled:        h.whatsappEnabled(),
	}

	h.renderPage(ctx, w, "render unified settings page", pages.UnifiedSettingsPage(data, lang, dir))
}
