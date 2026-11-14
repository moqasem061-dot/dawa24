package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// CompareMarketIntelligencePage renders the market intelligence dashboard.
func (h *UIHandler) CompareMarketIntelligencePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/market-intelligence", http.StatusSeeOther)
		return
	}
	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/customer/dashboard", "error", i18n.T(lang, "compare.intel.vendors_only"))
		return
	}
	if !actor.IsStaff && h.billSvc != nil {
		allowed, err := h.billSvc.CheckOrgEntitlement(ctx, actor.OrganizationID, actor.UserID, billing.FeatureMarketDiscounts)
		if err != nil || !allowed {
			h.redirectWithNotice(w, r, "/vendor/subscription?upgrade=pro", "error", i18n.T(lang, "compare.intel.upgrade_required"))
			return
		}
	}

	savingsQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(savingsQuery) > 80 {
		savingsQuery = savingsQuery[:80]
	}
	exclusiveQuery := strings.TrimSpace(r.URL.Query().Get("q_single"))
	if len(exclusiveQuery) > 80 {
		exclusiveQuery = exclusiveQuery[:80]
	}

	var (
		report        *compare.StrategicSavingReport
		reportErr     error
		orgPtr        *int64
		isAdminActing bool
	)
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}
	if actor.IsStaff || actor.IsPlatformAdmin() {
		if targetOrg, _ := strconv.ParseInt(r.URL.Query().Get("org_id"), 10, 64); targetOrg > 0 {
			orgPtr = &targetOrg
			isAdminActing = true
		}
	}

	if h.compareSvc != nil {
		report, reportErr = h.compareSvc.BuildStrategicReport(database.AsSystem(ctx), orgPtr, lang)
		if reportErr != nil {
			h.log.ErrorContext(ctx, "failed to build strategic saving report", "error", reportErr)
		}
	}
	if report != nil {
		report.TopSavings = filterSavingOpportunities(report.TopSavings, savingsQuery)
		report.Exclusives = filterSavingOpportunities(report.Exclusives, exclusiveQuery)
	}

	var targetOrgName string
	if isAdminActing && h.orgSvc != nil {
		sysCtx := database.WithTenant(database.AsSystem(ctx), *orgPtr)
		targetOrgName, _ = h.resolveTargetOrgInfo(sysCtx, *orgPtr)
	}

	pageData := pages.MarketIntelligencePageData{
		Report:     report,
		Failed:     reportErr != nil,
		IsCustomer: actor.IsCustomer(),
		IsAdmin:    isAdminActing,
		TargetOrgID: func() int64 {
			if orgPtr != nil {
				return *orgPtr
			}
			return 0
		}(),
		TargetOrgName: targetOrgName,
		Query:         savingsQuery,
		SingleQuery:   exclusiveQuery,
	}

	h.renderPage(ctx, w, "render market intelligence page", pages.CompareMarketIntelligencePage(lang, dir, pageData))
}

// filterSavingOpportunities filters intel-report rows by product name, SKU or
// supplier (both sides). Empty query returns the slice unchanged.
func filterSavingOpportunities(in []*compare.SavingOpportunity, q string) []*compare.SavingOpportunity {
	q = strings.TrimSpace(q)
	if q == "" {
		return in
	}
	needle := strings.ToLower(q)
	out := make([]*compare.SavingOpportunity, 0, len(in))
	for _, it := range in {
		if it == nil {
			continue
		}
		hay := strings.ToLower(it.ProductName + " " + it.SKU + " " + it.BestSupplier + " " + it.WorstSupplier)
		if strings.Contains(hay, needle) {
			out = append(out, it)
		}
	}
	return out
}
