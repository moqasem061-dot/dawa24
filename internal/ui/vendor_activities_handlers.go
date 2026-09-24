package ui

import (
	"net/http"

	platformadmin "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/pagination"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// VendorActivitiesPage renders the organization's employee activity logs.
func (h *UIHandler) VendorActivitiesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/activities", http.StatusSeeOther)
		return
	}

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	var entries []*platformadmin.AuditEntry
	var total int
	if h.adminSvc != nil {
		if actor.IsStaff || actor.Role == "super_admin" {
			if loaded, loaded2, err := h.adminSvc.ListAuditLogByOrgWithTotal(ctx, actor.OrganizationID, limit, offset); err != nil {
				h.logLoadErr(ctx, "adminSvc.ListAuditLogByOrgWithTotal", err)
			} else {
				entries, total = loaded, loaded2
			}
		} else {
			if loaded, loaded2, err := h.adminSvc.ListOrgStaffAuditLogWithTotal(ctx, actor.OrganizationID, limit, offset); err != nil {
				h.logLoadErr(ctx, "adminSvc.ListOrgStaffAuditLogWithTotal", err)
			} else {
				entries, total = loaded, loaded2
			}
		}
	}

	h.renderPage(ctx, w, "render vendor activities", pages.VendorActivitiesPage(entries, lang, dir, page, limit, total))
}

// CustomerActivitiesPage renders the pharmacy organization's employee activity logs.
func (h *UIHandler) CustomerActivitiesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/activities", http.StatusSeeOther)
		return
	}

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	var entries []*platformadmin.AuditEntry
	var total int
	if h.adminSvc != nil {
		if actor.IsStaff || actor.Role == "super_admin" {
			if loaded, loaded2, err := h.adminSvc.ListAuditLogByOrgWithTotal(ctx, actor.OrganizationID, limit, offset); err != nil {
				h.logLoadErr(ctx, "adminSvc.ListAuditLogByOrgWithTotal", err)
			} else {
				entries, total = loaded, loaded2
			}
		} else {
			if loaded, loaded2, err := h.adminSvc.ListOrgStaffAuditLogWithTotal(ctx, actor.OrganizationID, limit, offset); err != nil {
				h.logLoadErr(ctx, "adminSvc.ListOrgStaffAuditLogWithTotal", err)
			} else {
				entries, total = loaded, loaded2
			}
		}
	}

	h.renderPage(ctx, w, "render customer activities", pages.CustomerActivitiesPage(entries, lang, dir, page, limit, total))
}
