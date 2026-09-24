package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// VendorBranchDeleteSubmit deletes a branch scoped to the vendor organization.
func (h *UIHandler) VendorBranchDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", i18n.T(langOf(r), "vendor.branch.invalid_id_alt"))
		return
	}
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", i18n.T(langOf(r), "vendor.branch.orgs_service_unavailable"))
		return
	}
	existing, err := h.orgSvc.GetBranch(ctx, id)
	if err != nil || existing == nil || existing.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", i18n.T(langOf(r), "vendor.branch.not_found_edit"))
		return
	}
	if existing.IsMain {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", i18n.T(langOf(r), "customer.branch.cannot_delete_main"))
		return
	}
	if err := h.orgSvc.DeleteBranch(ctx, id, actor.OrganizationID); err != nil {
		h.log.ErrorContext(ctx, "delete branch", "error", err, "branch", id, "org", actor.OrganizationID)
		h.redirectWithNotice(w, r, "/vendor/branches", "error", h.safeMessage(err, langOf(r)))
		return
	}
	// The selector's option list is cached per organisation for a few seconds
	// (see branchCacheTTL); drop it so the person who just made this change
	// sees it on the next page rather than up to thirty seconds later.
	InvalidateBranchOptionsCache(actor.OrganizationID)
	h.redirectWithNotice(w, r, "/vendor/branches", "success", i18n.T(langOf(r), "customer.branch.delete_success"))
}
