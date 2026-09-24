package ui

import (
	"net/http"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/pagination"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// AdminUserAddressesPage renders address records for users.
func (h *UIHandler) AdminUserAddressesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		if loaded, err := h.idSvc.AdminListUsers(database.AsSystem(ctx), "", ""); err != nil {
			h.logLoadErr(ctx, "idSvc.AdminListUsers", err)
		} else {
			users = loaded
		}
	}

	h.renderPage(ctx, w, "render admin user addresses", pages.AdminFullUserPage(users, "addresses", lang, dir))
}

// AdminUserOrganizationPage renders user-to-organization membership directory.
func (h *UIHandler) AdminUserOrganizationPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	data := &pages.AdminUserOrgData{
		ActiveTab: statusFilter,
		Page:      page,
		PerPage:   limit,
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		list, total, err := h.orgSvc.ListAllUserOrganizationsWithTotal(sysCtx, statusFilter, limit, offset)
		if err != nil {
			h.logLoadErr(ctx, "orgSvc.ListAllUserOrganizationsWithTotal", err)
		} else {
			data.UserOrgs = list
			data.TotalCount = total
		}
	}

	h.renderPage(ctx, w, "render admin user organizations", pages.AdminUserOrganizationsPage(lang, dir, data))
}

// AdminWantDeletePage redirects account deletion requests to the unified deletion requests screen.
func (h *UIHandler) AdminWantDeletePage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/deletion-requests?tab=users", http.StatusMovedPermanently)
}
