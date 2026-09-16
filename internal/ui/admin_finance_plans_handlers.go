package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminPlansInfoPage renders subscription plan tiers directory.
func (h *UIHandler) AdminPlansInfoPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var plans []*billing.Plan
	if h.billSvc != nil {
		plans, _ = h.billSvc.ListPlans(ctx)
	}

	h.renderPage(ctx, w, "render admin plans info", pages.AdminPlansInfoPage(plans, lang, dir))
}

// AdminPlanTypesPage renders subscription plan types CRUD.
func (h *UIHandler) AdminPlanTypesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var items []pages.ReferenceItem
	if h.billSvc != nil {
		plans, _ := h.billSvc.ListPlans(ctx)
		for _, p := range plans {
			items = append(items, pages.ReferenceItem{
				ID:          p.ID,
				Name:        p.Name.Get(i18n.Lang(lang)),
				Description: p.Slug,
				Status:      "active",
				Extra:       p.Description.Get(i18n.Lang(lang)),
			})
		}
	}

	h.renderPage(ctx, w, "render admin plan types", pages.AdminReferenceCRUDPage(i18n.T(lang, "admin.finance.plan_types_title"), "plan-types", i18n.T(lang, "admin.finance.plan_type"), items, "plans", lang, dir))
}

// AdminPlanFeaturesPage renders feature matrix for subscription plans.
func (h *UIHandler) AdminPlanFeaturesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var items []pages.ReferenceItem
	if h.billSvc != nil {
		plans, _ := h.billSvc.ListPlans(ctx)
		for _, p := range plans {
			for k, v := range p.Features {
				items = append(items, pages.ReferenceItem{
					ID:          p.ID,
					Name:        k,
					Description: p.Name.Get(i18n.Lang(lang)),
					Status:      "active",
					Extra:       v,
				})
			}
		}
	}

	h.renderPage(ctx, w, "render admin plan features", pages.AdminReferenceCRUDPage(i18n.T(lang, "admin.finance.plan_features_title"), "plan-features", i18n.T(lang, "admin.finance.plan_feature"), items, "plans", lang, dir))
}

// AdminPlansSubscriptionsPage renders active subscriptions and subscriber histories.
func (h *UIHandler) AdminPlansSubscriptionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var subs []*billing.Subscription
	var total int
	if h.billSvc != nil {
		subs, total, _ = h.billSvc.AdminListSubscriptionsWithTotal(ctx, limit, offset)
	}

	h.renderPage(ctx, w, "render admin plan subscriptions", pages.AdminPlansSubscriptionsPage(lang, dir, pages.AdminPlansSubscriptionsPageData{
		Subscriptions: subs,
		Page:          page,
		PerPage:       limit,
		TotalCount:    total,
	}))
}
