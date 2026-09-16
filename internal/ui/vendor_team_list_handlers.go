package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorTeamPage renders the staff and RBAC roles configuration view.
func (h *UIHandler) VendorTeamPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/team", http.StatusSeeOther)
		return
	}

	noticeType := r.URL.Query().Get("notice_type")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice")
	}
	noticeMsg := r.URL.Query().Get("notice_msg")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("msg")
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var memberViews []*pages.TeamMemberView
	var branchOptions []*pages.BranchOption
	var totalCount int

	if h.orgSvc != nil && actor.OrganizationID > 0 {
		// 1. Fetch employees with full profiles
		if employees, total, err := h.orgSvc.ListEmployeesWithTotal(ctx, actor.OrganizationID, limit, offset); err == nil && len(employees) > 0 {
			totalCount = total
			for _, emp := range employees {
				roleName := emp.RoleName
				switch emp.Member.RoleKey {
				case "org_owner":
					roleName = i18n.T(lang, "role.org_owner")
				case "org_manager":
					roleName = i18n.T(lang, "role.org_manager")
				case "org_warehouse":
					roleName = i18n.T(lang, "role.org_warehouse")
				case "org_accountant":
					roleName = i18n.T(lang, "role.org_accountant")
				case "org_employee":
					roleName = i18n.T(lang, "role.org_employee")
				default:
					if roleName == "" {
						roleName = i18n.T(lang, "role.team_member")
					}
				}
				if emp.IsManager && emp.Member.RoleKey != "org_owner" {
					roleName = i18n.T(lang, "role.branch_manager")
				}

				name := emp.UserName
				if name == "" {
					name = emp.UserEmail
				}

				memberViews = append(memberViews, &pages.TeamMemberView{
					ID:           emp.Member.ID,
					UserID:       emp.Member.UserID,
					Name:         name,
					Email:        emp.UserEmail,
					Phone:        emp.UserPhone,
					AvatarURL:    emp.UserAvatar,
					JobTitle:     emp.Member.JobTitle,
					EmployeeCode: emp.Member.EmployeeCode,
					BranchID:     derefBranchID(emp.Member.BranchID),
					BranchName:   emp.BranchName,
					RoleKey:      emp.Member.RoleKey,
					RoleName:     roleName,
					RoleID:       derefRoleID(emp.Member.OrgRoleID),
					IsActive:     emp.Member.IsActive,
					CreatedAt:    emp.Member.CreatedAt.Format("2006-01-02"),
				})
			}
		} else {
			// Fallback to ListMembers if ListEmployees returns empty
			members, _ := h.orgSvc.ListMembers(ctx, actor.OrganizationID)
			for _, m := range members {
				name := i18n.T(lang, "role.employee")
				email := ""
				phone := ""
				avatarURL := ""
				if h.idSvc != nil {
					if u, err := h.idSvc.GetUserByID(ctx, m.UserID); err == nil && u != nil {
						name = u.Name.Get(i18n.AR)
						if name == "" {
							name = u.Name.Get(i18n.EN)
						}
						if name == "" {
							name = u.Email
						}
						email = u.Email
						phone = u.Phone
						avatarURL = u.AvatarURL
					}
				}
				roleName := i18n.T(lang, "role.org_employee")
				switch m.RoleKey {
				case "org_owner":
					roleName = i18n.T(lang, "role.org_owner")
				case "org_manager":
					roleName = i18n.T(lang, "role.org_manager")
				case "org_warehouse":
					roleName = i18n.T(lang, "role.org_warehouse")
				case "org_accountant":
					roleName = i18n.T(lang, "role.org_accountant")
				case "org_employee":
					roleName = i18n.T(lang, "role.org_employee")
				}
				memberViews = append(memberViews, &pages.TeamMemberView{
					ID:           m.ID,
					UserID:       m.UserID,
					Name:         name,
					Email:        email,
					Phone:        phone,
					AvatarURL:    avatarURL,
					JobTitle:     m.JobTitle,
					EmployeeCode: m.EmployeeCode,
					BranchID:     derefBranchID(m.BranchID),
					RoleKey:      m.RoleKey,
					RoleName:     roleName,
					RoleID:       derefRoleID(m.OrgRoleID),
					IsActive:     m.IsActive,
					CreatedAt:    m.CreatedAt.Format("2006-01-02"),
				})
			}
		}

		// 2. Fetch branches for the branch dropdown
		if branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil {
			for _, b := range branches {
				bName := b.Name.Get(i18n.AR)
				if bName == "" {
					bName = b.Name.Get(i18n.EN)
				}
				branchOptions = append(branchOptions, &pages.BranchOption{
					ID:   b.ID,
					Name: bName,
				})
			}
		}
	}

	data := pages.VendorTeamData{
		NoticeType:    noticeType,
		NoticeMsg:     noticeMsg,
		Members:       memberViews,
		Branches:      branchOptions,
		CanAssignRole: actor.Can("vendor.role.assign"),
		CurrentUserID: actor.UserID,
		Page:          page,
		PerPage:       limit,
		TotalCount:    totalCount,
	}
	// The assignable roles are this company's own. Resolving each member's
	// current role through the same list is what makes the selector show
	// where they stand rather than defaulting everyone to the first option.
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		h.ensureCompanyRoles(ctx, actor.OrganizationID, actor.OrgType)
		roles, err := h.orgSvc.ListRoles(ctx, actor.OrganizationID)
		if err != nil {
			h.log.ErrorContext(ctx, "list company roles for team page",
				"error", err, "organization_id", actor.OrganizationID)
		}
		roleIDByKey := map[string]int64{}
		roleNameByID := map[int64]string{}
		for _, role := range roles {
			if role == nil {
				continue
			}
			name := role.Name.Get(i18n.ParseLang(lang))
			if name == "" {
				name = role.Name.Get(i18n.AR)
			}
			if name == "" {
				name = role.Name.Get(i18n.EN)
			}
			if name == "" {
				name = role.Key
			}
			if !role.IsOwner && role.Key != "org_owner" {
				data.CompanyRoles = append(data.CompanyRoles, pages.TenantRoleOption{
					ID:      role.ID,
					Key:     role.Key,
					Name:    name,
					IsOwner: role.IsOwner,
				})
			}
			roleIDByKey[role.Key] = role.ID
			roleNameByID[role.ID] = name
		}
		for _, m := range memberViews {
			if m.RoleID == 0 {
				m.RoleID = roleIDByKey[m.RoleKey]
			}
			if dynName, ok := roleNameByID[m.RoleID]; ok && dynName != "" {
				m.RoleName = dynName
			}
		}
	}

	h.renderPage(ctx, w, "render vendor team page", pages.VendorTeamPage(data, lang, dir))
}

// derefRoleID unwraps the optional custom-role link on a membership. Zero
// means "no custom role assigned", and the caller falls back to the company's
// starter role for the member's role_key.
func derefRoleID(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}

// derefBranchID unwraps a member's optional branch assignment; zero means the
// employee is not tied to a specific branch.
func derefBranchID(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}

// VendorTeamImportPage renders bulk employee spreadsheet upload page.
func (h *UIHandler) VendorTeamImportPage(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportPage(w, r, "vendor")
}

// VendorTeamImportSampleDownload downloads the sample spreadsheet template for vendor team.
func (h *UIHandler) VendorTeamImportSampleDownload(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportSampleDownload(w, "vendor", "dawa24_vendor_team_sample.xlsx")
}

// VendorTeamImportSessionPage renders current session stage for vendor.
func (h *UIHandler) VendorTeamImportSessionPage(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportSessionPage(w, r, "vendor")
}

// VendorTeamFastAddPage renders fast add form for single employee account.
func (h *UIHandler) VendorTeamFastAddPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/team/fast-add", http.StatusSeeOther)
		return
	}

	h.renderPage(ctx, w, "render vendor team fast add", pages.VendorTeamFastAddPage(lang, dir))
}

// VendorTeamUserDetailPage renders single employee profile and assigned permissions.
func (h *UIHandler) VendorTeamUserDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	empID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || empID <= 0 {
		http.Redirect(w, r, "/settings/employees", http.StatusSeeOther)
		return
	}

	h.renderPage(ctx, w, "render vendor team user detail", pages.VendorTeamUserDetailPage(empID, lang, dir))
}

// VendorTeamUserInfoPage renders employee audit information.
func (h *UIHandler) VendorTeamUserInfoPage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, fmt.Sprintf("/vendor/team/%s", idStr), http.StatusSeeOther)
}
