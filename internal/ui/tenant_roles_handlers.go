package ui

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// The company role editor, /vendor/roles and /customer/roles.
//
// One implementation serves both dashboards. They differ only in which
// permissions may be granted, and that difference is data: the caller's
// organization type picks a scope, the scope picks the matrix, and the same
// scope is applied again on save so a forged form cannot reach past it.
//
// The screens these replace were static markup. /vendor/roles rendered three
// hardcoded cards describing roles that did not exist as rows, with no create,
// edit or delete; /customer/roles did not exist at all.

// tenantRoleContext resolves the caller's company and dashboard, and refuses
// anything it cannot establish from the session.
func (h *UIHandler) tenantRoleContext(r *http.Request) (actor authctx.Actor, scope rbac.Scope, base string, ok bool) {
	actor = authctx.FromContext(r.Context())
	if actor.OrganizationID <= 0 {
		return actor, "", "", false
	}
	scope, ok = rbac.TenantScopeFor(actor.OrgType)
	if !ok {
		return actor, "", "", false
	}
	if scope == rbac.ScopeVendor {
		return actor, scope, "/vendor/roles", true
	}
	return actor, scope, "/customer/roles", true
}

// VendorRolesPage lists the supplier company's roles.
func (h *UIHandler) VendorRolesPage(w http.ResponseWriter, r *http.Request) { h.tenantRolesPage(w, r) }

// CustomerRolesPage lists the pharmacy company's roles.
func (h *UIHandler) CustomerRolesPage(w http.ResponseWriter, r *http.Request) {
	h.tenantRolesPage(w, r)
}

// VendorRoleDetailPage opens one supplier role in the permission matrix.
func (h *UIHandler) VendorRoleDetailPage(w http.ResponseWriter, r *http.Request) {
	h.tenantRoleDetailPage(w, r)
}

// CustomerRoleDetailPage opens one pharmacy role in the permission matrix.
func (h *UIHandler) CustomerRoleDetailPage(w http.ResponseWriter, r *http.Request) {
	h.tenantRoleDetailPage(w, r)
}

// VendorRoleCreateSubmit adds a supplier role.
func (h *UIHandler) VendorRoleCreateSubmit(w http.ResponseWriter, r *http.Request) {
	h.tenantRoleCreate(w, r)
}

// CustomerRoleCreateSubmit adds a pharmacy role.
func (h *UIHandler) CustomerRoleCreateSubmit(w http.ResponseWriter, r *http.Request) {
	h.tenantRoleCreate(w, r)
}

// VendorRoleUpdateSubmit saves a supplier role.
func (h *UIHandler) VendorRoleUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	h.tenantRoleUpdate(w, r)
}

// CustomerRoleUpdateSubmit saves a pharmacy role.
func (h *UIHandler) CustomerRoleUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	h.tenantRoleUpdate(w, r)
}

// VendorRoleDeleteSubmit removes a supplier role.
func (h *UIHandler) VendorRoleDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	h.tenantRoleDelete(w, r)
}

// CustomerRoleDeleteSubmit removes a pharmacy role.
func (h *UIHandler) CustomerRoleDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	h.tenantRoleDelete(w, r)
}

// VendorMemberRoleAssignSubmit assigns a supplier employee to a role.
func (h *UIHandler) VendorMemberRoleAssignSubmit(w http.ResponseWriter, r *http.Request) {
	h.tenantMemberRoleAssign(w, r, "/vendor/team")
}

// CustomerMemberRoleAssignSubmit assigns a pharmacy employee to a role.
func (h *UIHandler) CustomerMemberRoleAssignSubmit(w http.ResponseWriter, r *http.Request) {
	h.tenantMemberRoleAssign(w, r, "/customer/team")
}

func (h *UIHandler) tenantRolesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, scope, base, ok := h.tenantRoleContext(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if h.orgSvc == nil {
		// The page exists and the caller may see it; the data behind it is
		// unavailable. Saying so beats a 404, which reads as "this feature is
		// not yours" (AGENTS.md: no fabricated success, no misleading absence).
		h.renderEmptyRoles(w, r, scope, base)
		return
	}

	// A company whose roles were never seeded — one that predates this system,
	// or was created while the seeder was not running — gets them now, so the
	// page is never an empty list that the owner cannot act on.
	h.ensureCompanyRoles(ctx, actor.OrganizationID, actor.OrgType)

	roles, err := h.orgSvc.ListRoles(ctx, actor.OrganizationID)
	if err != nil {
		h.log.ErrorContext(ctx, "list company roles", "error", err, "organization_id", actor.OrganizationID)
	}
	counts, err := h.orgSvc.RoleMemberCounts(ctx, actor.OrganizationID)
	if err != nil {
		h.log.WarnContext(ctx, "count role members", "error", err, "organization_id", actor.OrganizationID)
		counts = map[int64]int{}
	}

	view := pages.RolesView{
		Scope:      scope,
		BasePath:   base,
		Title:      i18n.T(lang, "tenant.roles.title"),
		Subtitle:   i18n.T(lang, "tenant.roles.subtitle"),
		CanCreate:  actor.Can(tenantPerm(scope, "role.create")),
		CanEdit:    actor.Can(tenantPerm(scope, "role.update")),
		CanDelete:  actor.Can(tenantPerm(scope, "role.delete")),
		NoticeKind: r.URL.Query().Get("notice"),
		Notice:     r.URL.Query().Get("msg"),
		Tenant:     true,
	}
	for _, role := range roles {
		view.Roles = append(view.Roles, pages.RoleRow{
			ID:          strconv.FormatInt(role.ID, 10),
			Name:        role.Name.Get(i18n.ParseLang(lang)),
			Description: role.Description,
			IsSystem:    role.IsSystem,
			IsOwner:     role.IsOwner,
			Badge:       tenantRoleBadge(role, lang),
			GrantCount:  grantCountFor(role, scope),
			MemberCount: counts[role.ID],
		})
	}

	h.renderPage(ctx, w, "render company roles page", pages.RolesPage(view, lang, dir))
}

func (h *UIHandler) tenantRoleDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, scope, base, ok := h.tenantRoleContext(r)
	if !ok || h.orgSvc == nil {
		http.NotFound(w, r)
		return
	}
	roleID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	// The organization id comes from the session, never from the URL. A role
	// id belonging to another company therefore reads as missing rather than
	// as forbidden, which is also all the caller is entitled to learn.
	role, err := h.orgSvc.GetRole(ctx, actor.OrganizationID, roleID)
	if err != nil {
		h.redirectWithNotice(w, r, base, "error", i18n.T(langOf(r), "tenant.roles.not_found"))
		return
	}

	view := pages.RoleEditView{
		Scope:       scope,
		BasePath:    base,
		ID:          strconv.FormatInt(role.ID, 10),
		Key:         role.Key,
		Name:        role.Name.Get(i18n.ParseLang(lang)),
		NameAr:      role.Name["ar"],
		NameEn:      role.Name["en"],
		Description: role.Description,
		IsSystem:    role.IsSystem,
		IsOwner:     role.IsOwner,
		ReadOnly:    role.IsOwner || !actor.Can(tenantPerm(scope, "role.update")),
		Granted:     rbac.NewSet(grantsFor(role, scope)),
		Sections:    rbac.Default().Matrix(scope),
		Tenant:      true,
	}
	if counts, err := h.orgSvc.RoleMemberCounts(ctx, actor.OrganizationID); err == nil {
		view.MemberCount = counts[role.ID]
	}

	h.renderPage(ctx, w, "render company role editor", pages.RoleEditPage(view, lang, dir))
}

func (h *UIHandler) tenantRoleCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, scope, base, ok := h.tenantRoleContext(r)
	if !ok || h.orgSvc == nil {
		http.NotFound(w, r)
		return
	}
	_ = r.ParseForm()

	role, err := h.orgSvc.CreateRole(ctx, org.RoleInput{
		OrganizationID: actor.OrganizationID,
		Scope:          scope,
		Key:            r.PostFormValue("key"),
		Name:           formName(r),
		Description:    r.PostFormValue("description"),
		Permissions:    r.Form["permissions"],
		CreatedBy:      actor.UserID,
	})
	if err != nil {
		h.redirectWithNotice(w, r, base, "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.invalidatePermissions(actor.UserID, actor.OrganizationID)
	h.redirectWithNotice(w, r, base+"/"+strconv.FormatInt(role.ID, 10),
		"success", i18n.T(langOf(r), "tenant.roles.created_success"))
}

func (h *UIHandler) tenantRoleUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, scope, base, ok := h.tenantRoleContext(r)
	if !ok || h.orgSvc == nil {
		http.NotFound(w, r)
		return
	}
	roleID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()

	if _, err := h.orgSvc.UpdateRole(ctx, roleID, org.RoleInput{
		OrganizationID: actor.OrganizationID,
		Scope:          scope,
		Name:           formName(r),
		Description:    r.PostFormValue("description"),
		Permissions:    r.Form["permissions"],
		CreatedBy:      actor.UserID,
	}); err != nil {
		h.redirectWithNotice(w, r, base+"/"+chi.URLParam(r, "id"), "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.invalidatePermissions(actor.UserID, actor.OrganizationID)
	h.redirectWithNotice(w, r, base+"/"+chi.URLParam(r, "id"), "success", i18n.T(langOf(r), "tenant.roles.saved_success"))
}

func (h *UIHandler) tenantRoleDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _, base, ok := h.tenantRoleContext(r)
	if !ok || h.orgSvc == nil {
		http.NotFound(w, r)
		return
	}
	roleID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := h.orgSvc.DeleteRole(ctx, actor.OrganizationID, roleID); err != nil {
		h.redirectWithNotice(w, r, base, "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.invalidatePermissions(actor.UserID, actor.OrganizationID)
	h.redirectWithNotice(w, r, base, "success", i18n.T(langOf(r), "tenant.roles.deleted_success"))
}

func (h *UIHandler) tenantMemberRoleAssign(w http.ResponseWriter, r *http.Request, base string) {
	ctx := r.Context()
	actor, _, _, ok := h.tenantRoleContext(r)
	if !ok || h.orgSvc == nil {
		http.NotFound(w, r)
		return
	}
	memberID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	roleID, _ := strconv.ParseInt(r.PostFormValue("role_id"), 10, 64)

	if err := h.orgSvc.AssignMemberRole(ctx, actor.OrganizationID, memberID, roleID); err != nil {
		h.redirectWithNotice(w, r, base, "error", h.safeMessage(err, langOf(r)))
		return
	}
	if m, err := h.orgSvc.GetMember(ctx, actor.OrganizationID, memberID); err == nil && m != nil && m.UserID > 0 {
		h.invalidatePermissions(m.UserID, actor.OrganizationID)
	}
	h.invalidatePermissions(actor.UserID, actor.OrganizationID)
	h.redirectWithNotice(w, r, base, "success", i18n.T(langOf(r), "tenant.roles.member_assigned_success"))
}

// renderEmptyRoles shows the roles screen with nothing in it and an
// explanation, for the case where the organization service is not wired.
func (h *UIHandler) renderEmptyRoles(w http.ResponseWriter, r *http.Request, scope rbac.Scope, base string) {
	lang, dir := h.localeAndDir(r)
	view := pages.RolesView{
		Scope:      scope,
		BasePath:   base,
		Title:      i18n.T(lang, "tenant.roles.title"),
		Subtitle:   i18n.T(lang, "tenant.roles.empty_subtitle"),
		Tenant:     true,
		NoticeKind: "error",
		Notice:     i18n.T(lang, "tenant.roles.service_unavailable"),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.RolesPage(view, lang, dir).Render(r.Context(), w); err != nil {
		h.log.ErrorContext(r.Context(), "render empty roles page", "error", err)
	}
}

// grantsFor returns a role's stored grants, substituting the whole dashboard
// for the owner role — whose authority is implied rather than stored, so a
// literal read would show it holding nothing.
func grantsFor(role *org.Role, scope rbac.Scope) []string {
	if role.IsOwner {
		return rbac.Default().KeysFor(scope)
	}
	return role.Permissions
}

func grantCountFor(role *org.Role, scope rbac.Scope) int {
	return len(grantsFor(role, scope))
}

func tenantRoleBadge(role *org.Role, lang any) string {
	switch {
	case role.IsOwner:
		return i18n.T(lang, "tenant.roles.badge_owner")
	case role.IsSystem:
		return i18n.T(lang, "tenant.roles.badge_system")
	default:
		return i18n.T(lang, "tenant.roles.badge_custom")
	}
}

// tenantPerm builds a scoped permission key: "role.update" becomes
// "vendor.role.update" or "pharmacy.role.update" depending on the dashboard.
func tenantPerm(scope rbac.Scope, suffix string) string {
	return string(scope) + "." + suffix
}

// ensureCompanyRoles seeds the starter roles for a company that has none.
func (h *UIHandler) ensureCompanyRoles(ctx context.Context, orgID int64, orgType string) {
	if h.roleSeeder == nil || orgID <= 0 {
		return
	}
	if err := h.roleSeeder(ctx, orgID, orgType); err != nil {
		h.log.WarnContext(ctx, "could not seed company roles",
			"error", err, "organization_id", orgID, "org_type", orgType)
	}
}
