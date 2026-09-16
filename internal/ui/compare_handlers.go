package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/features"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// ComparePlansPage renders the pricing page for the discount-comparison plans.
func (h *UIHandler) ComparePlansPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect="+r.URL.RequestURI(), http.StatusSeeOther)
		return
	}
	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/customer/dashboard", "error", i18n.T(lang, "compare.plans.vendors_only"))
		return
	}
	if !features.Enabled(ctx, "compare.enabled") {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	var viewPlans []*billing.Plan
	if h.compareSvc != nil {
		cPlans, err := h.compareSvc.ListPlans(ctx, true)
		if err == nil {
			for _, cp := range cPlans {
				viewPlans = append(viewPlans, &billing.Plan{
					ID:          cp.ID,
					Name:        cp.Name,
					Slug:        cp.Slug,
					Description: cp.Description,
					PriceMonth:  cp.PriceMonthly,
					PriceYear:   cp.PriceYearly,
					IsActive:    cp.IsActive,
				})
			}
		}
	} else if h.billSvc != nil {
		viewPlans, _ = h.billSvc.ListPlans(ctx)
	}

	h.renderPage(ctx, w, "render compare plans", pages.ComparePlansPage(lang, dir, viewPlans))
}

// CompareSubscribeSubmit subscribes the caller to a compare plan.
func (h *UIHandler) CompareSubscribeSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare", http.StatusSeeOther)
		return
	}
	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/customer/dashboard", "error", i18n.T(lang, "compare.plans.vendors_only"))
		return
	}

	slug := r.URL.Query().Get("plan")
	if slug == "" {
		h.redirectWithNotice(w, r, "/compare", "error", i18n.T(lang, "compare.plans.subscribe_failed"))
		return
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	if h.compareSvc != nil {
		if _, err := h.compareSvc.SubscribeDirectly(ctx, slug, orgPtr, actor.UserID, "monthly"); err != nil {
			h.redirectWithNotice(w, r, "/compare", "error", h.safeMessage(err, lang))
			return
		}
	} else if h.billSvc != nil {
		if _, err := h.billSvc.Subscribe(ctx, actor.UserID, orgPtr, slug, "compare", nil); err != nil {
			h.redirectWithNotice(w, r, "/compare", "error", h.safeMessage(err, lang))
			return
		}
	}
	h.redirectWithNotice(w, r, "/compare/tool", "success", i18n.T(lang, "compare.plans.subscribed_success"))
}

// CompareToolPage renders the 3-column comparison workspace.
func (h *UIHandler) CompareToolPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/customer/dashboard", "error", i18n.T(lang, "compare.tool.vendors_only"))
		return
	}
	if !actor.IsStaff && h.billSvc != nil {
		allowed, err := h.billSvc.CheckOrgEntitlement(ctx, actor.OrganizationID, actor.UserID, billing.FeatureCompareTool)
		if err != nil || !allowed {
			h.redirectWithNotice(w, r, "/vendor/subscription?upgrade=pro", "error", i18n.T(lang, "compare.tool.upgrade_required"))
			return
		}
	}

	noticeType := r.URL.Query().Get("notice")
	noticeMsg := r.URL.Query().Get("msg")

	// Subscription Feature Gate Check
	if !actor.IsStaff && h.billSvc != nil {
		allowed, err := h.billSvc.CheckOrgEntitlement(ctx, actor.OrganizationID, actor.UserID, billing.FeatureCompareTool)
		if err != nil || !allowed {
			plans, _ := h.billSvc.ListPlans(ctx)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if err := pages.SubscriptionGatePage(lang, dir, pages.SubscriptionGateProps{
				FeatureKey:   billing.FeatureCompareTool,
				FeatureTitle: i18n.T(lang, "compare.tool.gate_title"),
				FeatureDesc:  i18n.T(lang, "compare.tool.gate_desc"),
				FeatureIcon:  "📊",
				Plans:        plans,
				Actor:        actor,
			}).Render(ctx, w); err != nil {
				h.log.ErrorContext(ctx, "render subscription gate page", "error", err)
			}
			return
		}
	}

	// Archived files are deliberately not read here. A bulk import replaces the
	// vendor's previous lists, and what it replaced is no longer part of their
	// workspace: it stays in the database, in full, for the Super Admin's
	// temporary-warehouses screen, under its archive filter.
	var activeFiles []*compare.CompareFile
	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}
	if h.compareSvc != nil {
		activeFiles, _ = h.compareSvc.ListFiles(ctx, actor.UserID, orgPtr, nil)
	}

	maxAllowedFiles := 10
	if h.billSvc != nil {
		if plan, pErr := h.billSvc.GetEffectivePlan(ctx, actor.UserID, orgPtr); pErr == nil && plan != nil {
			maxAllowedFiles = plan.GetMaxCompareFiles()
		}
	}

	h.renderPage(ctx, w, "render compare tool", pages.CompareToolPage(pages.CompareToolView{
		Lang:            lang,
		Dir:             dir,
		Files:           activeFiles,
		MaxAllowedFiles: maxAllowedFiles,
		NoticeType:      noticeType,
		NoticeMsg:       noticeMsg,
	}))
}
