package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// The supplier's branch-quota screen.
//
// Three actions: set or lift the cap on an item, reset one branch's
// consumption, and undo a reset. All three are POSTs behind
// vendor.quota.manage; the page itself only needs vendor.quota.view, so a
// warehouse keeper can see who has taken what without being able to hand out
// more.

// VendorQuotasPage renders the quota dashboard.
func (h *UIHandler) VendorQuotasPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/quotas", http.StatusSeeOther)
		return
	}

	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab != "variants" {
		tab = "branches"
	}

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)

	filter := commerce.QuotaFilter{
		Query:         strings.TrimSpace(r.URL.Query().Get("q")),
		VariantID:     queryID(r, "variant"),
		CustomerOrgID: queryID(r, "customer"),
		BranchID:      queryID(r, "branch"),
		State:         quotaState(r.URL.Query().Get("state")),
		Limit:         limit,
		Offset:        (page - 1) * limit,
	}

	data := pages.VendorQuotasData{
		ActiveTab:     tab,
		Query:         filter.Query,
		VariantID:     filter.VariantID,
		CustomerOrgID: filter.CustomerOrgID,
		BranchID:      filter.BranchID,
		State:         filter.State,
		Page:          page,
		PerPage:       limit,
		CanManage:     actor.Can("vendor.quota.manage"),
	}

	if h.commSvc != nil && actor.OrganizationID > 0 {
		summary, err := h.commSvc.QuotaSummary(ctx, actor.OrganizationID)
		if err != nil {
			h.renderError(w, r, err)
			return
		}
		data.Summary = summary

		variants, customers, branches, err := h.commSvc.QuotaFilterOptions(ctx, actor.OrganizationID)
		if err != nil {
			h.renderError(w, r, err)
			return
		}
		data.VariantOptions, data.CustomerOptions, data.BranchOptions = variants, customers, branches

		// The two tabs page independently; loading both would make the row
		// count on screen disagree with the pager on whichever tab is hidden.
		if tab == "variants" {
			rows, total, err := h.commSvc.ListQuotaVariantRows(ctx, actor.OrganizationID, filter)
			if err != nil {
				h.renderError(w, r, err)
				return
			}
			data.VariantRows, data.TotalCount = rows, total
		} else {
			rows, total, err := h.commSvc.ListBranchQuotaRows(ctx, actor.OrganizationID, filter)
			if err != nil {
				h.renderError(w, r, err)
				return
			}
			data.BranchRows, data.TotalCount = rows, total
		}
	}

	h.renderPage(ctx, w, "render vendor quotas page", pages.VendorQuotas(data, lang, dir))
}

// VendorQuotaLimitSubmit sets or lifts the per-branch cap on one variant.
//
// An empty quota_limit is the remove-the-quota action rather than a validation
// error: the same field the supplier types a number into is the one they clear
// to remove the restriction, and the two must be one form.
func (h *UIHandler) VendorQuotaLimitSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/quotas", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		h.log.WarnContext(ctx, "parse quota form", "error", err)
	}
	back := quotaReturnTo(r, "variants")

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	if variantID <= 0 || h.catSvc == nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "vendor.catalog.invalid_quota"))
		return
	}

	limit, err := parseQuotaLimit(r.PostFormValue("quota_limit"), lang)
	if err != nil {
		h.redirectWithNotice(w, r, back, "error", err.Error())
		return
	}

	if err := h.catSvc.SetVariantQuotaLimit(ctx, variantID, limit); err != nil {
		h.log.ErrorContext(ctx, "set variant quota", "error", err,
			"variant", variantID, "org", actor.OrganizationID)
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, lang))
		return
	}

	key := "vendor.quota.limit_removed"
	if limit != nil {
		key = "vendor.quota.limit_saved"
	}
	h.redirectWithNotice(w, r, back, "success", i18n.Translate(lang, key))
}

// VendorQuotaReleaseSubmit resets one branch's consumption of one variant.
func (h *UIHandler) VendorQuotaReleaseSubmit(w http.ResponseWriter, r *http.Request) {
	h.quotaReleaseAction(w, r, false)
}

// VendorQuotaReleaseUndoSubmit restores a consumption the supplier had reset.
func (h *UIHandler) VendorQuotaReleaseUndoSubmit(w http.ResponseWriter, r *http.Request) {
	h.quotaReleaseAction(w, r, true)
}

// quotaReleaseAction is the shared body of release and undo. The two differ
// only in which service call they make and which message they report, and
// writing them out twice is how the ownership check comes to exist on one of
// them.
func (h *UIHandler) quotaReleaseAction(w http.ResponseWriter, r *http.Request, undo bool) {
	ctx := r.Context()
	lang := langOf(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/quotas", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		h.log.WarnContext(ctx, "parse quota release form", "error", err)
	}
	back := quotaReturnTo(r, "branches")

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	branchID, _ := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	if h.commSvc == nil || actor.OrganizationID <= 0 || variantID <= 0 || branchID <= 0 {
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(commerce.ErrQuotaTargetInvalid, lang))
		return
	}

	var err error
	if undo {
		err = h.commSvc.UndoBranchQuotaRelease(ctx, actor.OrganizationID, variantID, branchID)
	} else {
		err = h.commSvc.ReleaseBranchQuota(ctx, actor.OrganizationID, variantID, branchID,
			actor.UserID, strings.TrimSpace(r.PostFormValue("note")))
	}
	if err != nil {
		h.log.ErrorContext(ctx, "quota release", "error", err, "undo", undo,
			"variant", variantID, "branch", branchID, "org", actor.OrganizationID)
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, lang))
		return
	}

	key := "vendor.quota.release_done"
	if undo {
		key = "vendor.quota.undo_done"
	}
	h.redirectWithNotice(w, r, back, "success", i18n.Translate(lang, key))
}

// quotaReturnTo picks where to send the supplier after an action: back to the
// filtered page they acted from, and never to a URL the form invented.
func quotaReturnTo(r *http.Request, tab string) string {
	back := strings.TrimSpace(r.PostFormValue("return_to"))
	if strings.HasPrefix(back, "/vendor/quotas") {
		return back
	}
	return "/vendor/quotas?tab=" + tab
}

// queryID reads an optional positive identifier out of the query string.
func queryID(r *http.Request, key string) int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get(key)), 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

// quotaState keeps a hand-edited state filter inside the set the query
// understands, rather than pasting it into SQL.
func quotaState(raw string) string {
	switch strings.TrimSpace(raw) {
	case commerce.QuotaStateExhausted:
		return commerce.QuotaStateExhausted
	case commerce.QuotaStateActive:
		return commerce.QuotaStateActive
	case commerce.QuotaStateReleased:
		return commerce.QuotaStateReleased
	default:
		return ""
	}
}
