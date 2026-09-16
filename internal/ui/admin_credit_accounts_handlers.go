package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// The administrator's per-organization view of package credits.

// AdminOffersPackagesOrganizationsPage lists every company holding packages.
func (h *UIHandler) AdminOffersPackagesOrganizationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	page, perPage := creditPaging(r)

	accounts, total, err := h.promoSvc.CreditAccounts(ctx, search, perPage, (page-1)*perPage)
	if err != nil {
		h.log.ErrorContext(ctx, "list credit accounts", "error", err)
		h.renderError(w, r, err)
		return
	}

	h.renderPage(ctx, w, "package accounts", pages.AdminCreditAccountsPage(
		pages.AdminCreditAccountsView{
			Lang:     lang,
			Accounts: accounts,
			Search:   search,
			Page:     page,
			PerPage:  perPage,
			Total:    total,
		}, lang, dir))
}

// AdminOffersPackagesOrganizationDetailPage lists one company's purchases, each
// linking to the same كشف حساب the company itself can open.
func (h *UIHandler) AdminOffersPackagesOrganizationDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	orgID := parseInt64PathParam(r, "id")
	if orgID <= 0 {
		http.NotFound(w, r)
		return
	}
	page, perPage := creditPaging(r)

	purchases, total, err := h.promoSvc.PurchasesForOrg(ctx, orgID, perPage, (page-1)*perPage)
	if err != nil {
		h.log.ErrorContext(ctx, "list org purchases", "org_id", orgID, "error", err)
		h.renderError(w, r, err)
		return
	}

	h.renderPage(ctx, w, "organization packages", pages.AdminOrgPackagesPage(
		pages.AdminOrgPackagesView{
			Lang:      lang,
			OrgID:     orgID,
			OrgName:   h.organizationDisplayName(r, orgID),
			Purchases: purchases,
			Page:      page,
			PerPage:   perPage,
			Total:     total,
		}, lang, dir))
}

// creditPaging reads the shared page/limit pair, clamped so a hand-edited URL
// cannot ask for an unbounded scan.
func creditPaging(r *http.Request) (page, perPage int) {
	page, perPage = 1, 25
	if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && n > 0 {
		page = n
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 100 {
		perPage = n
	}
	return page, perPage
}
