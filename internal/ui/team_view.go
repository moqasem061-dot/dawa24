package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// Building the team view, once, for the screen both audiences will share.
//
// The pharmacy and the supplier each had their own team page over the same
// org.members rows, and the pharmacy had two — a thin one at /customer/team and
// a much better one hidden behind a tab on /customer/branches. This assembles
// the view the surviving screen renders.

// fillTenantTeamView loads the members, the company's own roles and its
// branches into a view.
func (h *UIHandler) fillTenantTeamView(
	ctx context.Context, view *pages.TenantTeamView,
	orgID int64, orgType, lang string, limit, offset int,
) {
	if h.orgSvc == nil {
		return
	}

	// The company's roles have to exist before anyone can be assigned one. A
	// company created before the role catalogue landed has none, and the add
	// dialog would otherwise offer an empty dropdown.
	h.ensureCompanyRoles(ctx, orgID, orgType)

	roleNameByID := map[int64]string{}
	roleIDByKey := map[string]int64{}
	if roles, err := h.orgSvc.ListRoles(ctx, orgID); err != nil {
		h.log.ErrorContext(ctx, "team view: list company roles", "error", err, "organization_id", orgID)
	} else {
		for _, role := range roles {
			if role == nil {
				continue
			}
			name := role.Name.Get(i18n.ParseLang(lang))
			if name == "" {
				name = role.Key
			}
			if !role.IsOwner && role.Key != "org_owner" {
				view.Roles = append(view.Roles, pages.TenantRoleOption{
					ID: role.ID, Key: role.Key, Name: name, IsOwner: role.IsOwner,
				})
			}
			roleNameByID[role.ID] = name
			roleIDByKey[role.Key] = role.ID
		}
	}

	branchNameByID := map[int64]string{}
	if branches, err := h.orgSvc.ListBranches(ctx, orgID); err != nil {
		h.log.ErrorContext(ctx, "team view: list branches", "error", err, "organization_id", orgID)
	} else {
		for _, b := range branches {
			if b == nil {
				continue
			}
			name := b.Name.Get(i18n.AR)
			if name == "" {
				name = b.Name.Get(i18n.EN)
			}
			view.Branches = append(view.Branches, &pages.BranchOption{ID: b.ID, Name: name})
			branchNameByID[b.ID] = name
		}
	}

	employees, total, err := h.orgSvc.ListEmployeesWithTotal(ctx, orgID, limit, offset)
	if err != nil {
		h.log.ErrorContext(ctx, "team view: list employees", "error", err, "organization_id", orgID)
		return
	}
	view.TotalCount = total

	for _, emp := range employees {
		if emp == nil || emp.Member == nil {
			continue
		}
		view.Members = append(view.Members, teamMemberRow(emp, roleNameByID, roleIDByKey, branchNameByID))
	}
}

// teamMemberRow flattens one membership into the row the table renders.
func teamMemberRow(
	emp *org.EmployeeView,
	roleNameByID map[int64]string, roleIDByKey map[string]int64,
	branchNameByID map[int64]string,
) pages.TenantTeamMember {
	m := emp.Member

	// A member carrying only the legacy role_key still shows the company's own
	// role for it rather than an empty cell.
	roleID := int64(0)
	if m.OrgRoleID != nil && *m.OrgRoleID > 0 {
		roleID = *m.OrgRoleID
	} else if id, ok := roleIDByKey[m.RoleKey]; ok {
		roleID = id
	}
	roleName := roleNameByID[roleID]
	if roleName == "" {
		roleName = m.RoleKey
	}

	branchID := int64(0)
	if m.BranchID != nil && *m.BranchID > 0 {
		branchID = *m.BranchID
	}
	branchName := branchNameByID[branchID]
	if branchName == "" {
		branchName = emp.BranchName
	}

	name := strings.TrimSpace(emp.UserName)
	if name == "" {
		name = emp.UserEmail
	}

	return pages.TenantTeamMember{
		ID:           m.ID,
		UserID:       m.UserID,
		Name:         name,
		Email:        emp.UserEmail,
		Phone:        emp.UserPhone,
		JobTitle:     m.JobTitle,
		EmployeeCode: m.EmployeeCode,
		BranchID:     branchID,
		BranchName:   branchName,
		RoleID:       roleID,
		RoleKey:      m.RoleKey,
		RoleName:     roleName,
		IsActive:     m.IsActive,
		IsManager:    emp.IsManager,
		AvatarURL:    emp.UserAvatar,
		JoinedAt:     m.CreatedAt.Format("2006-01-02"),
	}
}

// parseInt64Param reads a positive integer from the query string, or zero.
func parseInt64Param(r *http.Request, key string) int64 {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// resolveTeamRole turns whatever the form submitted — a company role id, or a
// legacy role_key — into both, so org.members.org_role_id and role_key stay in
// step. They are two columns describing one thing and the screens used to write
// whichever they happened to carry.
func (h *UIHandler) resolveTeamRole(ctx context.Context, orgID int64, roleIDRaw, roleKeyRaw string) (int64, string) {
	roleKey := strings.TrimSpace(roleKeyRaw)
	if roleKey == "org_admin" || roleKey == "org_owner" {
		roleKey = "org_employee"
	}

	roleID, _ := strconv.ParseInt(strings.TrimSpace(roleIDRaw), 10, 64)
	if h.orgSvc == nil {
		return roleID, defaultRoleKey(roleKey)
	}

	roles, err := h.orgSvc.ListRoles(ctx, orgID)
	if err != nil {
		h.log.ErrorContext(ctx, "resolve team role: list roles", "error", err, "organization_id", orgID)
		return roleID, defaultRoleKey(roleKey)
	}

	for _, role := range roles {
		if role == nil || role.IsOwner || role.Key == "org_owner" {
			continue
		}
		if roleID > 0 && role.ID == roleID {
			return role.ID, role.Key
		}
		if roleID <= 0 && roleKey != "" && role.Key == roleKey {
			return role.ID, role.Key
		}
	}

	// A role id that names nothing in this company or attempts to assign owner
	// is not assignable. Fall back to the ordinary member role rather than writing it.
	for _, role := range roles {
		if role != nil && !role.IsOwner && role.Key != "org_owner" {
			return role.ID, role.Key
		}
	}
	return 0, defaultRoleKey(roleKey)
}

func defaultRoleKey(key string) string {
	if key == "" || key == "org_owner" || key == "org_admin" {
		return "org_employee"
	}
	return key
}

// resolveTeamBranch accepts only a branch of the caller's own company.
func (h *UIHandler) resolveTeamBranch(ctx context.Context, orgID int64, raw string) *int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 || h.orgSvc == nil {
		return nil
	}
	branches, err := h.orgSvc.ListBranches(ctx, orgID)
	if err != nil {
		h.log.ErrorContext(ctx, "resolve team branch: list branches", "error", err, "organization_id", orgID)
		return nil
	}
	for _, b := range branches {
		if b != nil && b.ID == id {
			return &id
		}
	}
	return nil
}
