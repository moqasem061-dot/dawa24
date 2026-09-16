package ui

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminDashboardPage renders the platform's headline numbers.
//
// The numbers come from a cached snapshot that is computed off the request
// path — see admin_dashboard_cache.go for why that is not optional here. This
// handler's whole job is to ask for the snapshot and localise it.
func (h *UIHandler) AdminDashboardPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor := authctx.FromContext(ctx)

	snap := adminDashboard.get(ctx, h.computeDashboardSnapshot)
	if snap == nil {
		// Nothing cached and the first computation did not finish inside its
		// budget. Render the page empty rather than hold the request open: the
		// refresh is still running and the next load will have it.
		snap = &dashboardSnapshot{}
	}

	stats, pendingOrgs := dashboardStatsFor(snap, lang)
	h.renderPage(ctx, w, "render admin dashboard page",
		pages.AdminDashboard(stats, pendingOrgs, actor, lang, dir))
}

// dashboardStatsFor projects a snapshot into the page's view model, formatting
// money and localised text for this request's language.
func dashboardStatsFor(s *dashboardSnapshot, lang string) (pages.AdminDashboardStats, []*org.Organization) {
	currency := i18n.T(lang, "common.currency_egp")

	stats := pages.AdminDashboardStats{
		TotalUsers:            s.totalUsers,
		TotalOrganizations:    s.totalOrganizations,
		TotalPharmacies:       s.totalPharmacies,
		TotalVendors:          s.totalVendors,
		TotalBranches:         s.totalBranches,
		PendingApprovals:      s.pendingApprovals,
		PendingDepositsCount:  s.pendingDepositsCount,
		PendingDepositsAmount: fmt.Sprintf("%s %s", money.FromMinor(s.pendingDepositsMinor).String(), currency),
		TotalHeldInWallets:    fmt.Sprintf("%s %s", money.FromMinor(s.heldInWalletsMinor).String(), currency),
		TotalOrders:           s.totalOrders,
		ActiveOrdersCount:     s.activeOrders,
		CompletedOrdersCount:  s.completedOrders,
		TotalProducts:         s.totalProducts,
		TotalSavingProducts:   s.totalSavingProducts,
		TotalGMV:              s.totalGMV,
		TotalVisitors:         s.totalVisitors,
		TodayVisitors:         s.todayVisitors,
		TopDevices:            nonNilCounts(s.topDevices),
		TopBrowsers:           nonNilCounts(s.topBrowsers),
		TopLocations:          nonNilCounts(s.topLocations),
		GatewayOnline:         s.gatewayOnline,
		RecentOrders:          s.recentOrders,
		RecentOrganizations:   s.recentOrganizations,
	}

	if s.hasGMV && !strings.HasPrefix(s.totalGMV, "0.00") && s.totalGMV != "0" {
		stats.TotalCommission = i18n.T(lang, "admin.dashboard.commission_5pct")
	} else {
		stats.TotalCommission = fmt.Sprintf("0.00 %s", currency)
	}

	return stats, s.pendingOrganizations
}

func nonNilCounts(m map[string]int) map[string]int {
	if m == nil {
		return map[string]int{}
	}
	return m
}

// computeDashboardSnapshot gathers every figure the dashboard shows.
//
// The groups below are independent — they touch different schemas and share no
// state — so they run concurrently. Each one still opens its own transactions;
// what changed is that the request no longer pays for them, and they no longer
// run once per page view.
func (h *UIHandler) computeDashboardSnapshot(ctx context.Context) *dashboardSnapshot {
	sysCtx := database.AsSystem(ctx)
	s := &dashboardSnapshot{
		topDevices:   map[string]int{},
		topBrowsers:  map[string]int{},
		topLocations: map[string]int{},
	}

	identityGroup := func() {
		if h.idSvc == nil {
			return
		}
		if n, err := h.idSvc.AdminCountUsers(ctx); err == nil {
			s.totalUsers = n
		}
	}

	orgGroup := func() {
		if h.orgSvc == nil {
			return
		}
		if n, err := h.orgSvc.CountOrganizations(ctx, nil, nil); err == nil {
			s.totalOrganizations = n
		}
		pharmacyType := org.TypeCustomer
		if n, err := h.orgSvc.CountOrganizations(ctx, &pharmacyType, nil); err == nil {
			s.totalPharmacies = n
		}
		vendorType := org.TypeVendor
		if n, err := h.orgSvc.CountOrganizations(ctx, &vendorType, nil); err == nil {
			s.totalVendors = n
		}
		pending := org.StatusPending
		if n, err := h.orgSvc.CountOrganizations(ctx, nil, &pending); err == nil {
			s.pendingApprovals = n
		}
		if list, err := h.orgSvc.ListOrganizations(ctx, nil, &pending, 6, 0); err == nil {
			s.pendingOrganizations = list
		}
		if list, err := h.orgSvc.ListOrganizations(ctx, nil, nil, 6, 0); err == nil {
			s.recentOrganizations = list
		}
		// AdminBranchStats and not ListBranches(0): the latter selects every
		// branch on the platform, joined to identity.users with a correlated
		// array_agg per row, and materialises the lot in Go so that len() can
		// be taken of it. The count is a count.
		if bs, err := h.orgSvc.AdminBranchStats(sysCtx); err == nil {
			s.totalBranches = bs.TotalBranches
		}
	}

	commerceGroup := func() {
		if h.commSvc == nil {
			return
		}
		if n, err := h.commSvc.CountOrders(ctx); err == nil {
			s.totalOrders = n
		}
		if recent, _, err := h.commSvc.AdminSearchOrdersFiltered(ctx, commerce.AdminOrderFilter{Tab: "all", Limit: 8}); err == nil {
			s.recentOrders = recent
			for _, o := range recent {
				if o.Status == commerce.StatusDelivered || o.Status == commerce.StatusCompleted {
					s.completedOrders++
				} else if o.Status != commerce.StatusCancelled && o.Status != commerce.StatusFailed {
					s.activeOrders++
				}
			}
		}
	}

	billingGroup := func() {
		if h.billSvc == nil {
			return
		}
		if deps, total, err := h.billSvc.AdminListDetailedDeposits(sysCtx,
			billing.DepositFilter{Status: "pending", Limit: 50}); err == nil {
			s.pendingDepositsCount = total
			var depTotal money.Amount
			for _, d := range deps {
				depTotal, _ = depTotal.Add(d.Amount)
			}
			s.pendingDepositsMinor = depTotal.Minor()
		}
		if wallets, _, err := h.billSvc.AdminListDetailedWallets(sysCtx,
			billing.WalletFilter{Limit: 100}); err == nil {
			for _, wlt := range wallets {
				s.heldInWalletsMinor += wlt.Balance.Minor()
			}
		}
	}

	catalogGroup := func() {
		if h.catSvc == nil {
			return
		}
		if _, savingStats, err := h.catSvc.ListAllSavingProductsAdmin(
			sysCtx, nil, nil, "", "all", 1, 0); err == nil && savingStats != nil {
			s.totalSavingProducts = savingStats.TotalProducts
		}
	}

	analyticsGroup := func() {
		if h.adminSvc == nil {
			return
		}
		if va, err := h.adminSvc.VisitorAnalytics(ctx, 10); err == nil && va != nil {
			s.totalVisitors = va.Total
			s.todayVisitors = va.Today
			s.topDevices = va.ByDevice
			s.topBrowsers = va.ByBrowser
			s.topLocations = va.ByCity
			s.totalGMV = va.TotalGMV
			s.hasGMV = va.TotalGMV != ""
			s.totalProducts = va.TotalProducts
		}
	}

	runParallel(identityGroup, orgGroup, commerceGroup, billingGroup, catalogGroup, analyticsGroup)

	// The budget ran out, so most of these numbers are zeros that never came
	// back rather than figures. Returning nil keeps the last good snapshot in
	// place instead of replacing a real dashboard with an empty one.
	if ctx.Err() != nil {
		return nil
	}

	// Credentials only; this builds no connection and makes no network call.
	if gwAdmin, _, ok := h.getGatewayAdminClient(ctx); ok && gwAdmin != nil {
		s.gatewayOnline = true
	}

	return s
}
