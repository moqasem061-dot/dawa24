package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminFullUserPage renders the centralized user management index.
func (h *UIHandler) AdminFullUserPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		// AsSystem justified: platform administrator browsing users across all tenants
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "", "")
	}

	h.renderPage(ctx, w, "render admin full user page", pages.AdminFullUserPage(users, "all", lang, dir))
}

// AdminNewClientsPage renders recent new customer registrations.
func (h *UIHandler) AdminNewClientsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "customer", "")
	}

	h.renderPage(ctx, w, "render admin new clients", pages.AdminFullUserPage(users, "new_clients", lang, dir))
}

// AdminCustomerListPage renders pharmacy customer accounts.
func (h *UIHandler) AdminCustomerListPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "customer", "")
	}

	h.renderPage(ctx, w, "render admin customers", pages.AdminFullUserPage(users, "customers", lang, dir))
}

// AdminVendorListPage renders vendor and supplier user accounts.
func (h *UIHandler) AdminVendorListPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "vendor", "")
	}

	h.renderPage(ctx, w, "render admin vendors", pages.AdminFullUserPage(users, "vendors", lang, dir))
}

// AdminStaffListPage renders platform administrators and staff users.
func (h *UIHandler) AdminStaffListPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "staff", "")
	}

	h.renderPage(ctx, w, "render admin staff", pages.AdminFullUserPage(users, "staff", lang, dir))
}

// AdminUserDetailPage renders detailed user profile with role and organization info.
func (h *UIHandler) AdminUserDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	userID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || userID <= 0 {
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	var user *identity.User
	if h.idSvc != nil {
		user, _ = h.idSvc.GetUserByID(sysCtx, userID)
	}

	if user == nil {
		h.redirectWithNotice(w, r, "/admin/users", "error", i18n.T(lang, "admin.users.not_found"))
		return
	}

	actor := authctx.FromContext(ctx)
	view := pages.AdminUserDetailView{
		User:           user,
		SelfEdit:       actor.UserID == user.ID,
		CanEditUser:    actor.Can("identity.user.update"),
		CanSetPassword: actor.Can("identity.user.update"),
		CanAssignRole:  actor.Can("identity.admin_role.assign"),
		Notice:         r.URL.Query().Get("notice"),
		NoticeKind:     r.URL.Query().Get("kind"),
	}

	// 1. National ID
	if h.idSvc != nil {
		if nid, err := h.idSvc.GetNationalID(sysCtx, user.ID); err == nil {
			view.NationalID = nid
		}
	}

	// 2. Organizations & Branches lookup
	var allOrgs []*org.Organization
	var allBranches []*org.Branch
	orgNames := make(map[int64]string)
	branchNames := make(map[int64]string)

	if h.orgSvc != nil {
		if orgs, err := h.orgSvc.ListOrganizations(sysCtx, nil, nil, 500, 0); err == nil {
			allOrgs = orgs
			for _, o := range orgs {
				if o != nil {
					orgNames[o.ID] = o.LegalName
				}
			}
		}
		if branches, _, err := h.orgSvc.ListBranchesWithTotal(sysCtx, org.BranchFilter{Status: "active"}, 1000, 0); err == nil {
			allBranches = branches
			for _, b := range branches {
				if b != nil {
					branchNames[b.ID] = b.Name.Get(i18n.ParseLang(lang))
				}
			}
		}
	}
	view.Organizations = allOrgs
	view.Branches = allBranches

	// 3. User Memberships and Member details
	var userOrgs []*identity.UserOrgMembership
	if h.idSvc != nil {
		userOrgs, _ = h.idSvc.ListUserOrganizations(sysCtx, user.ID)
	}
	view.Memberships = userOrgs

	var memberDetails []pages.AdminMemberDetailView
	for _, uo := range userOrgs {
		if uo == nil {
			continue
		}
		det := pages.AdminMemberDetailView{
			OrganizationID:   uo.OrganizationID,
			OrganizationName: uo.OrgName.Get(i18n.ParseLang(lang)),
			OrgType:          uo.OrgType,
			OrgStatus:        uo.OrgStatus,
			RoleKey:          uo.RoleKey,
			IsActive:         uo.IsActive,
		}
		if det.OrganizationName == "" && orgNames[uo.OrganizationID] != "" {
			det.OrganizationName = orgNames[uo.OrganizationID]
		}
		if h.orgSvc != nil {
			if members, err := h.orgSvc.ListMembers(sysCtx, uo.OrganizationID); err == nil {
				for _, m := range members {
					if m != nil && m.UserID == user.ID {
						det.JobTitle = m.JobTitle
						if m.BranchID != nil {
							det.BranchID = m.BranchID
							det.BranchName = branchNames[*m.BranchID]
						}
						break
					}
				}
			}
		}
		memberDetails = append(memberDetails, det)
	}
	view.MemberDetails = memberDetails
	if len(memberDetails) > 0 {
		view.PrimaryMember = &memberDetails[0]
	}

	// 4. Platform Roles assignable
	if actor.Can("identity.admin_role.assign") && h.idSvc != nil {
		roles, err := h.idSvc.ListPlatformRoles(ctx)
		if err != nil {
			h.log.ErrorContext(ctx, "list platform roles for user detail", "error", err)
		}
		for _, role := range roles {
			view.Roles = append(view.Roles, pages.AdminUserRoleOption{
				Key:      role.Key,
				Name:     role.Name.Get(i18n.ParseLang(lang)),
				IsStaff:  role.IsStaff,
				Selected: role.Key == user.Role,
			})
		}
	}

	// 5. Moderator hierarchy
	view.IsModerator = user.Role == identity.RoleModerator
	view.CanAssignModerator = actor.Can("identity.moderator.assign")
	if view.IsModerator && view.CanAssignModerator && h.idSvc != nil {
		if mods, err := h.idSvc.ListModerators(sysCtx); err == nil {
			for _, m := range mods {
				if m == nil {
					continue
				}
				if m.UserID == user.ID {
					view.ModeratorParentID = m.ParentID
					view.SubordinateCount = m.SubordinateCount
					continue
				}
				if !m.IsMain() {
					continue
				}
				view.MainModerators = append(view.MainModerators, pages.AdminModeratorOption{
					UserID: m.UserID,
					Name:   m.Name.Get(i18n.ParseLang(lang)),
					Team:   m.SubordinateCount,
				})
			}
		} else {
			h.log.ErrorContext(ctx, "list moderators for user detail", "error", err)
		}
	}

	// 6. Effective Permissions resolved by rbac.Resolver
	if h.resolver != nil {
		platformGrant, _ := h.resolver.Resolve(sysCtx, user.ID, 0)

		orgGrants := make(map[int64]rbac.Grant)
		for _, uo := range userOrgs {
			if uo != nil && uo.OrganizationID > 0 {
				og, err := h.resolver.Resolve(sysCtx, user.ID, uo.OrganizationID)
				if err == nil {
					orgGrants[uo.OrganizationID] = og
				}
			}
		}

		catalog := rbac.Default()
		groups := catalog.Groups()
		perms := catalog.Permissions()

		permsByGroup := make(map[string][]rbac.Permission)
		for _, p := range perms {
			permsByGroup[p.Group] = append(permsByGroup[p.Group], p)
		}

		totalPermsCount := 0
		totalHeldCount := 0
		var permGroups []pages.AdminEffectivePermGroup

		for _, g := range groups {
			groupPerms := permsByGroup[g.Key]
			if len(groupPerms) == 0 {
				continue
			}

			groupView := pages.AdminEffectivePermGroup{
				Key:    g.Key,
				NameAr: g.NameAr,
				NameEn: g.NameEn,
			}

			for _, p := range groupPerms {
				totalPermsCount++
				item := pages.AdminEffectivePermItem{
					Key:    p.Key,
					NameAr: p.NameAr,
					NameEn: p.NameEn,
					Kind:   string(p.Kind),
				}

				if platformGrant.Can(p.Key) || platformGrant.IsPlatformOwner {
					item.Held = true
					item.Source = "منصة (Platform)"
				} else {
					for orgID, og := range orgGrants {
						if og.Can(p.Key) || og.IsOrgOwner {
							item.Held = true
							orgName := orgNames[orgID]
							if orgName == "" {
								orgName = fmt.Sprintf("منشأة #%d", orgID)
							}
							item.Source = orgName
							break
						}
					}
				}

				if item.Held {
					groupView.HeldCount++
					totalHeldCount++
				}
				groupView.TotalCount++
				groupView.Permissions = append(groupView.Permissions, item)
			}

			permGroups = append(permGroups, groupView)
		}

		view.EffectivePermGroups = permGroups
		view.TotalPermsCount = totalPermsCount
		view.TotalHeldCount = totalHeldCount
	}

	h.renderPage(ctx, w, "render admin user detail page", pages.AdminUserDetailPage(view, lang, dir))
}

// AdminUserAddressesPage renders address records for users.
func (h *UIHandler) AdminUserAddressesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "", "")
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
		if err == nil {
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
