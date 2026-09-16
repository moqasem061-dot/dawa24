package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// SettingsEmployeesRedirect sends an old /settings/employees link to the team
// page for the caller's audience.
//
// There were three employee screens over org.members: this one, /customer/team
// and a tab on /customer/branches. Each had its own create form and its own
// delete, and they did not agree on whether {id} meant a member or a user — so
// deleting from one of them removed the wrong person. One screen per audience
// survives, and the routes that fed the other two are gone rather than gated,
// because a write path nobody can see is a write path nobody maintains.
func (h *UIHandler) SettingsEmployeesRedirect(w http.ResponseWriter, r *http.Request) {
	target := "/customer/team"
	if actor, ok := authctx.From(r.Context()); ok && actor.IsVendor() {
		target = "/vendor/team"
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// SettingsBranchManagerAssignSubmit names one member as a branch's manager.
//
// It survives the removal of /settings/employees because /vendor/branches posts
// to it: the branch screen is where a manager is chosen, not the team screen.
// It now returns to the branch list rather than to a page that no longer exists.
func (h *UIHandler) SettingsBranchManagerAssignSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	back := "/customer/branches"
	if actor.IsVendor() {
		back = "/vendor/branches"
	}
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "common.service_unavailable"))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "common.invalid_form_data"))
		return
	}

	branchID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if branchID <= 0 {
		branchID, _ = strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	}
	if branchID <= 0 {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "settings.emp.invalid_branch_id"))
		return
	}
	// The branch has to be this company's; the id arrives from the URL.
	if h.resolveTeamBranch(ctx, actor.OrganizationID, strconv.FormatInt(branchID, 10)) == nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "settings.emp.invalid_branch_id"))
		return
	}

	var managerUserID *int64
	if mStr := strings.TrimSpace(r.PostFormValue("manager_user_id")); mStr != "" && mStr != "0" {
		if mID, err := strconv.ParseInt(mStr, 10, 64); err == nil && mID > 0 {
			managerUserID = &mID
		}
	}

	if err := h.orgSvc.AssignBranchManager(ctx, actor.OrganizationID, branchID, managerUserID); err != nil {
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "settings.emp.manager_assigned_success"))
}

// PaymentMethodsRedirect sends an old /settings/payment-methods write to the
// wallet screen that now owns it.
//
// Those routes sat in the approved-only tier, which has no permission gate: a
// member holding neither pharmacy.wallet.manage nor vendor.wallet.manage could
// add, edit and delete the company's payment details through them, while the
// wallet page they were looking at offered no edit form at all. The writes moved
// beside the wallet; this keeps a stale form or bookmark from silently failing.
func (h *UIHandler) PaymentMethodsRedirect(w http.ResponseWriter, r *http.Request) {
	target := "/customer/wallet"
	if actor, ok := authctx.From(r.Context()); ok && actor.IsVendor() {
		target = "/vendor/wallet"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
