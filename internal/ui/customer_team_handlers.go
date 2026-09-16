package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CustomerTeamPage is the pharmacy's single team screen.
//
// It renders what the branches page used to hide behind a second tab: the
// branch assignment, the employee code, the search and branch filters, and the
// add and edit dialogs. /customer/branches is now branches.
func (h *UIHandler) CustomerTeamPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/team", http.StatusSeeOther)
		return
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)

	view := pages.TenantTeamView{
		Title:         i18n.T(lang, "customer.team.title"),
		RolesPath:     "/customer/roles",
		ImportPath:    "/customer/team/import",
		ActionBase:    "/customer/employees",
		CanCreate:     actor.Can("pharmacy.team.create"),
		CanUpdate:     actor.Can("pharmacy.team.update"),
		CanDelete:     actor.Can("pharmacy.team.delete"),
		CanAssign:     actor.Can("pharmacy.role.assign"),
		NoticeKind:    r.URL.Query().Get("notice"),
		Notice:        r.URL.Query().Get("msg"),
		FocusBranch:   parseInt64Param(r, "branch"),
		CurrentUserID: actor.UserID,
		Page:          page,
		PerPage:       limit,
	}
	h.fillTenantTeamView(ctx, &view, actor.OrganizationID, actor.OrgType, lang, limit, (page-1)*limit)

	h.renderPage(ctx, w, "render pharmacy team page", pages.TenantTeamPage(view, lang, dir))
}

// CustomerTeamImportPage renders bulk employee spreadsheet upload page for pharmacies.
func (h *UIHandler) CustomerTeamImportPage(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportPage(w, r, "customer")
}

// CustomerTeamImportUploadSubmit handles file upload and creates new employee import session for pharmacy.
func (h *UIHandler) CustomerTeamImportUploadSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportUploadSubmit(w, r, "customer")
}

// CustomerTeamImportSampleDownload downloads sample spreadsheet template for pharmacy team.
func (h *UIHandler) CustomerTeamImportSampleDownload(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportSampleDownload(w, "customer", "dawa24_pharmacy_team_sample.xlsx")
}

// CustomerTeamImportSessionPage renders current session stage for pharmacy.
func (h *UIHandler) CustomerTeamImportSessionPage(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportSessionPage(w, r, "customer")
}

// CustomerTeamImportMapSubmit saves user column mappings and role mappings for pharmacy.
func (h *UIHandler) CustomerTeamImportMapSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportMapSubmit(w, r, "customer")
}

// CustomerTeamImportCommitSubmit creates user accounts and links employees to pharmacy org.
func (h *UIHandler) CustomerTeamImportCommitSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportCommitSubmit(w, r, "customer", "org_pharmacist")
}

// CustomerTeamImportCancelSubmit cancels and deletes an import session for pharmacy.
func (h *UIHandler) CustomerTeamImportCancelSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportCancelSubmit(w, r, "customer")
}
