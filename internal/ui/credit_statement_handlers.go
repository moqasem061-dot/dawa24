package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// كشف حساب للباقة.
//
// One handler serves the vendor's own statement and the administrator's view of
// someone else's. The difference is which tenant the read runs under: a vendor
// reads inside their own tenant so the service refuses another company's
// purchase id outright, and an administrator reads as the system, which is what
// lets the admin drill-down exist at all.

// CreditStatementPage renders one purchase's credit ledger.
func (h *UIHandler) CreditStatementPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	purchaseID := parseInt64PathParam(r, "id")
	if purchaseID <= 0 {
		http.NotFound(w, r)
		return
	}

	isAdmin := strings.HasPrefix(r.URL.Path, "/admin/")
	readCtx := ctx
	if isAdmin {
		readCtx = database.AsSystem(ctx)
	} else if actor, ok := authctx.From(ctx); ok && actor.OrganizationID > 0 {
		readCtx = database.WithTenant(ctx, actor.OrganizationID)
	}

	page, perPage := 1, 25
	if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && n > 0 {
		page = n
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 100 {
		perPage = n
	}

	statement, total, err := h.promoSvc.CreditStatementFor(
		readCtx, purchaseID, perPage, (page-1)*perPage)
	if err != nil {
		h.log.ErrorContext(ctx, "read credit statement",
			"purchase_id", purchaseID, "error", err)
		h.renderError(w, r, err)
		return
	}

	view := pages.CreditStatementView{
		Lang:       lang,
		Statement:  statement,
		BackURL:    creditStatementBackURL(isAdmin),
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		NoticeKind: r.URL.Query().Get("notice_type"),
		Notice:     r.URL.Query().Get("notice_msg"),
	}
	// An administrator reading someone else's statement needs to be told whose
	// it is; a vendor reading their own does not.
	if isAdmin && statement.Purchase != nil {
		view.OrgName = h.organizationDisplayName(r, statement.Purchase.OrganizationID)
	}

	h.renderPage(ctx, w, "credit statement", pages.CreditStatementPage(view, lang, dir))
}

func creditStatementBackURL(isAdmin bool) string {
	if isAdmin {
		return "/admin/offers-packages/organizations"
	}
	return "/vendor/offers-packages"
}

// organizationDisplayName resolves one company's name the way every other list
// on the platform does: trade name first, legal name as the fallback.
func (h *UIHandler) organizationDisplayName(r *http.Request, orgID int64) string {
	o, err := h.orgSvc.GetOrganization(database.AsSystem(r.Context()), orgID)
	if err != nil || o == nil {
		return strconv.FormatInt(orgID, 10)
	}
	if display := o.TradeName.Get(i18n.ParseLang(langOf(r))); display != "" {
		return display
	}
	if o.LegalName != "" {
		return o.LegalName
	}
	return strconv.FormatInt(orgID, 10)
}
