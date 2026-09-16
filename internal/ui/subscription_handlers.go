package ui

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) loadOrgSubscriptionView(ctx context.Context, actor authctx.Actor, lang string) *pages.OrgSubscriptionView {
	if actor.OrganizationID <= 0 && actor.UserID <= 0 {
		return nil
	}

	subView := &pages.OrgSubscriptionView{
		HasSubscription:  true,
		PlanName:         i18n.T(lang, "sub.default_plan_name"),
		PlanSlug:         "basic",
		Status:           i18n.T(lang, "sub.status_active"),
		ExpiresAt:        i18n.T(lang, "sub.auto_renews"),
		MaxLoginSessions: 3,
		MaxDevices:       3,
		AIPlanID:         gateway.FallbackPlanID,
		IsDefaultPlan:    true,
	}

	sysCtx := database.AsSystem(ctx)

	var sub *billing.Subscription

	// Fetch Subscription & Plan
	if h.billSvc != nil {
		if actor.OrganizationID > 0 {
			sub, _ = h.billSvc.GetActiveSubscriptionByOrg(sysCtx, actor.OrganizationID)
		}
		if sub == nil && actor.UserID > 0 {
			sub, _ = h.billSvc.GetActiveSubscription(sysCtx, actor.UserID)
		}

		if sub != nil {
			subView.Status = i18n.T(lang, "sub.status_active")
			if !sub.ExpiresAt.IsZero() {
				subView.ExpiresAt = sub.ExpiresAt.Format("2006-01-02")
				subView.RawExpiresAt = sub.ExpiresAt
				now := time.Now()
				diff := sub.ExpiresAt.Sub(now)
				subView.HasDaysRemaining = true
				if diff > 0 {
					subView.DaysRemaining = int(diff.Hours() / 24)
					if subView.DaysRemaining == 0 {
						subView.DaysRemaining = 1
					}
				} else {
					subView.DaysRemaining = 0
				}
			}
			if plan, err := h.billSvc.GetPlanByID(sysCtx, sub.PlanID); err == nil && plan != nil {
				subView.PlanName = plan.Name.Get(i18n.Lang(lang))
				subView.PlanSlug = plan.Slug
				subView.MaxLoginSessions = plan.MaxLoginSessions
				subView.MaxDevices = plan.MaxDevices
				subView.AIPlanID = plan.AIPlanID
				subView.IsDefaultPlan = plan.IsDefault
				if sub.BillingCycle == "annual" {
					subView.RenewalCost = plan.PriceYear
				} else {
					subView.RenewalCost = plan.PriceMonth
				}
				if plan.IsDefault {
					subView.HasDaysRemaining = false
				}
			}
		} else {
			if defPlan, err := h.billSvc.GetDefaultPlan(sysCtx); err == nil && defPlan != nil {
				subView.PlanName = defPlan.Name.Get(i18n.Lang(lang))
				subView.PlanSlug = defPlan.Slug
				subView.MaxLoginSessions = defPlan.MaxLoginSessions
				subView.MaxDevices = defPlan.MaxDevices
				subView.AIPlanID = defPlan.AIPlanID
				subView.IsDefaultPlan = true
				subView.HasDaysRemaining = false
			}
		}
	}

	subView.BaseLoginSessions = subView.MaxLoginSessions

	// Fetch Org AI credentials, Extra Sessions & Live Consumption
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		h.EnsureOrgAIGatewayProvisioned(ctx, actor.OrganizationID)
		if o, err := h.orgSvc.GetOrganization(sysCtx, actor.OrganizationID); err == nil && o != nil {
			if activeExtra := o.ActiveExtraDevices(); activeExtra > 0 {
				subView.ExtraLoginSessions = activeExtra
				subView.HasExtraSessions = true
				subView.ExtraSessionsExpires = o.ExtraDevicesExpiresAt
				if o.ExtraDevicesExpiresAt != nil {
					subView.ExtraSessionsExpiresAt = o.ExtraDevicesExpiresAt.Format("2006-01-02")
				}
				subView.MaxLoginSessions += activeExtra
				subView.MaxDevices += activeExtra
			}

			subView.AIUserID = o.AIUserID
			if subView.AIUserID == "" {
				subView.AIUserID = fmt.Sprintf("org-%d", actor.OrganizationID)
			}
			if len(o.AIVirtualKey) > 8 {
				subView.AIVirtualKeyMasked = o.AIVirtualKey[:4] + "••••••••" + o.AIVirtualKey[len(o.AIVirtualKey)-4:]
			} else if o.AIVirtualKey != "" {
				subView.AIVirtualKeyMasked = "••••••••"
			}

			// Consumption comes from our own ledger: it is complete, it is
			// not capped at one API page, and it is still there when the
			// Gateway is not. This used to be a live HTTP call to
			// api.muhiya.com on every dashboard render.
			if h.aiUsage != nil {
				since := time.Now().Add(-aiLedgerWindow)
				if sub != nil && !sub.StartsAt.IsZero() && sub.StartsAt.After(since) {
					since = sub.StartsAt
				}
				if summary, err := h.aiUsage.Summarize(ctx, actor.OrganizationID, since); err == nil {
					subView.AIRequestsCount = summary.Requests
					subView.AITokensUsed = int(summary.TotalTokens())
					subView.AIBudgetSpentUSD = summary.CostUSD()
					subView.HasAIUsage = true
				} else {
					h.log.WarnContext(ctx, "could not summarise ai usage for dashboard",
						"org_id", actor.OrganizationID, "error", err)
				}
			}

			// The budget window is the one thing the Gateway alone knows: it
			// owns the plan's ceiling and the moment it reopens. Asked for
			// nothing else.
			if adminClient, _, ok := h.getGatewayAdminClient(ctx); ok && adminClient != nil {
				if userDetail, err := adminClient.GetUser(ctx, subView.AIUserID); err == nil && userDetail != nil && len(userDetail.BudgetUsage) > 0 {
					bw := userDetail.BudgetUsage[0]
					subView.AIBudgetName = bw.Name
					subView.AIBudgetLimitUSD = bw.BudgetUSD
					// If the subscription was upgraded/reset recently and local requests since StartsAt is 0,
					// reset displayed spent to 0 until new requests occur.
					if sub != nil && !sub.StartsAt.IsZero() && subView.AIRequestsCount == 0 && time.Since(sub.StartsAt) < 30*24*time.Hour {
						subView.AIBudgetSpentUSD = 0.0
					} else {
						subView.AIBudgetSpentUSD = bw.CurrentSpent
					}
					subView.HasAIUsage = true
					if !bw.ResetTime.IsZero() {
						subView.AIBudgetResetTime = bw.ResetTime.Format("2006-01-02 03:04 PM")
					}
				}
			}
		}
	}

	// Nothing below invents a number.
	//
	// What used to be here manufactured a budget ceiling from the plan slug
	// ($200 / $50 / $15), manufactured a reset date as the first of next month,
	// and then set HasAIUsage true regardless — so every card drew a confident
	// percentage out of two values the Gateway had never supplied. A pharmacy
	// deciding whether to upgrade was reading fiction.
	//
	// The Gateway is the only authority on both figures. When it has not
	// published them the flags stay false and the screens say so.
	if subView.AIBudgetSpentUSD < 0 {
		subView.AIBudgetSpentUSD = 0
	}
	subView.HasAIBudget = subView.AIBudgetLimitUSD > 0

	return subView
}

// TenantSubscriptionPage renders the dedicated subscription and membership page for pharmacies and vendors.
func (h *UIHandler) TenantSubscriptionPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || (actor.OrganizationID <= 0 && actor.UserID <= 0) {
		http.Redirect(w, r, "/auth/login?redirect="+r.URL.Path, http.StatusSeeOther)
		return
	}

	lang, dir := h.localeAndDir(r)
	subView := h.loadOrgSubscriptionView(ctx, actor, lang)

	if subView != nil && subView.HasDaysRemaining && !subView.IsDefaultPlan && subView.DaysRemaining <= 7 && subView.DaysRemaining >= 0 && actor.UserID > 0 {
		h.maybeNotifySubscriptionExpiring(ctx, actor, subView)
	}

	var allPlans []*billing.Plan
	var currentPlanID int64
	var walletBal money.Amount
	var autoRenew bool
	var billingCycle string = "monthly"
	var dataCooldownActive bool
	var dataCooldownUntil time.Time
	var dataCooldownDays int
	var dataCooldownHours int
	sysCtx := database.AsSystem(ctx)

	if h.billSvc != nil {
		if plans, err := h.billSvc.ListPlans(sysCtx); err == nil && len(plans) > 0 {
			allPlans = plans
		} else {
			if err != nil {
				h.log.ErrorContext(ctx, "failed to list billing plans", "error", err)
			}
			if adminPlans, aErr := h.billSvc.AdminListPlans(sysCtx); aErr == nil && len(adminPlans) > 0 {
				for _, ap := range adminPlans {
					if ap.IsActive {
						allPlans = append(allPlans, ap)
					}
				}
			}
		}

		if actor.OrganizationID > 0 {
			if sub, _ := h.billSvc.GetActiveSubscriptionByOrg(sysCtx, actor.OrganizationID); sub != nil {
				currentPlanID = sub.PlanID
				autoRenew = sub.AutoRenew
				if sub.BillingCycle != "" {
					billingCycle = sub.BillingCycle
				}
			}
		}
		if currentPlanID <= 0 && actor.UserID > 0 {
			if sub, _ := h.billSvc.GetActiveSubscription(sysCtx, actor.UserID); sub != nil {
				currentPlanID = sub.PlanID
				autoRenew = sub.AutoRenew
				if sub.BillingCycle != "" {
					billingCycle = sub.BillingCycle
				}
			}
		}

		// Always ensure wallet exists for seamless in-app upgrades
		walletUserID, _ := resolveTenantUserIDs(ctx, h, actor)
		if walletUserID > 0 {
			if w, err := h.billSvc.GetWallet(sysCtx, walletUserID, "EGP"); err == nil && w != nil {
				walletBal = w.Available()
			}
		}

		var orgPtr *int64
		if actor.OrganizationID > 0 {
			orgPtr = &actor.OrganizationID
		}
		cooldownInfo, _ := h.billSvc.CheckSubscriptionChangeCooldown(sysCtx, walletUserID, orgPtr, "")
		dataCooldownActive = cooldownInfo.IsOnCooldown
		dataCooldownUntil = cooldownInfo.EarliestAllowedAt
		dataCooldownDays = cooldownInfo.CooldownDays
		dataCooldownHours = cooldownInfo.CooldownHours
	}

	orgType := actor.OrgType
	if orgType == "" {
		if actor.IsVendor() {
			orgType = "vendor"
		} else {
			orgType = "customer"
		}
	}

	cooldownUntilFormatted := ""
	if !dataCooldownUntil.IsZero() {
		cooldownUntilFormatted = dataCooldownUntil.Format("2006-01-02 15:04")
	}

	data := pages.TenantSubscriptionPageData{
		Subscription:      subView,
		Plans:             allPlans,
		CurrentPlanID:     currentPlanID,
		WalletBalance:     walletBal,
		AutoRenew:         autoRenew,
		BillingCycle:      billingCycle,
		NoticeType:        r.URL.Query().Get("notice_type"),
		NoticeMsg:         r.URL.Query().Get("notice"),
		CooldownActive:    dataCooldownActive,
		CooldownUntil:     dataCooldownUntil,
		CooldownUntilText: cooldownUntilFormatted,
		CooldownDays:      dataCooldownDays,
		CooldownHours:     dataCooldownHours,
	}

	h.renderPage(ctx, w, "render subscription page", pages.TenantSubscriptionPage(data, orgType, lang, dir))
}

// OnboardingPendingPage renders the approval gate for pending/rejected/suspended/under_review orgs.
func (h *UIHandler) OnboardingPendingPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	state := r.URL.Query().Get("state")
	if actor, ok := authctx.From(ctx); ok {
		if actor.IsStaff {
			http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
			return
		}
		if actor.OrgStatus == "approved" || actor.OrgStatus == "active" || actor.OrgStatus == "verified" {
			http.Redirect(w, r, landingPathForActor(actor), http.StatusSeeOther)
			return
		}
		if state == "" && actor.OrgStatus != "" {
			state = actor.OrgStatus
		}
	}
	if r.URL.Query().Get("type") == "job_seeker" {
		state = "job_seeker"
	} else if state == "" {
		if r.URL.Query().Get("rejected") == "1" {
			state = "rejected"
		} else {
			state = "pending"
		}
	}
	lang, dir := h.localeAndDir(r)

	h.renderPage(ctx, w, "render onboarding pending", pages.OnboardingPending(lang, dir, state))
}

// maybeNotifySubscriptionExpiring checks if the user has already received an expiration warning notification
// for the current period, and dispatches an in-app notification if not. Guarantees zero spam (fires strictly once per cycle).
func (h *UIHandler) maybeNotifySubscriptionExpiring(ctx context.Context, actor authctx.Actor, subView *pages.OrgSubscriptionView) {
	if h.notifSvc == nil || actor.UserID <= 0 || subView == nil {
		return
	}

	sysCtx := database.AsSystem(ctx)
	// Check back 14 days to prevent duplicate notifications during the 7-day expiration countdown
	since := time.Now().Add(-14 * 24 * time.Hour)

	titleAr := "تنبيه: اقتراب انتهاء باقة الاشتراك"
	titleEn := "Subscription Expiring Soon"

	if exists, err := h.notifSvc.HasNotificationWithTitle(sysCtx, actor.UserID, titleAr, since); err == nil && exists {
		return
	}
	if exists, err := h.notifSvc.HasNotificationWithTitle(sysCtx, actor.UserID, titleEn, since); err == nil && exists {
		return
	}

	daysLeft := subView.DaysRemaining
	var bodyAr string
	switch daysLeft {
	case 0:
		bodyAr = fmt.Sprintf("سينتهي اشتراكك في باقة %s اليوم. يرجى تجديد الاشتراك أو شحن المحفظة لتجنب توقف الميزات.", subView.PlanName)
	case 1:
		bodyAr = fmt.Sprintf("سينتهي اشتراكك في باقة %s خلال يوم واحد. يرجى تجديد الاشتراك أو شحن المحفظة لتجنب توقف الميزات.", subView.PlanName)
	case 2:
		bodyAr = fmt.Sprintf("سينتهي اشتراكك في باقة %s خلال يومين. يرجى تجديد الاشتراك أو شحن المحفظة لتجنب توقف الميزات.", subView.PlanName)
	default:
		bodyAr = fmt.Sprintf("سينتهي اشتراكك في باقة %s خلال %d أيام. يرجى تجديد الاشتراك أو شحن المحفظة لتجنب توقف الميزات.", subView.PlanName, daysLeft)
	}

	var orgIDPtr *int64
	if actor.OrganizationID > 0 {
		orgIDPtr = &actor.OrganizationID
	}

	h.dispatchInAppNotification(sysCtx, actor.UserID, orgIDPtr, "", titleAr, bodyAr)
}

