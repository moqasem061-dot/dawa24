package ui

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminUsersPage renders the dedicated user management directory.
func (h *UIHandler) AdminUsersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)
	lang, dir := h.localeAndDir(r)

	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	roleFilter := strings.TrimSpace(r.URL.Query().Get("role"))
	typeFilter := strings.TrimSpace(r.URL.Query().Get("type"))
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	mfaFilter := strings.TrimSpace(r.URL.Query().Get("mfa"))
	loginFromFilter := strings.TrimSpace(r.URL.Query().Get("login_from"))
	loginToFilter := strings.TrimSpace(r.URL.Query().Get("login_to"))
	orgIDFilter, _ := strconv.ParseInt(r.URL.Query().Get("org_id"), 10, 64)

	var mfaBool *bool
	if mfaFilter == "1" || mfaFilter == "enabled" || mfaFilter == "true" {
		t := true
		mfaBool = &t
	} else if mfaFilter == "0" || mfaFilter == "disabled" || mfaFilter == "false" {
		f := false
		mfaBool = &f
	}

	var loginFromTime, loginToTime *time.Time
	if loginFromFilter != "" {
		if t, err := time.Parse("2006-01-02", loginFromFilter); err == nil {
			loginFromTime = &t
		}
	}
	if loginToFilter != "" {
		if t, err := time.Parse("2006-01-02", loginToFilter); err == nil {
			endDay := t.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
			loginToTime = &endDay
		}
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var users []*identity.User
	var totalCount int
	var allOrgs []*org.Organization
	orgNames := make(map[int64]string)
	var allBranches []*org.Branch
	var deletionRequests []*identity.AccountDeletionRequest
	var totalUsers, customerUsers, vendorUsers, staffUsers, activeUsers, suspendedUsers int

	if h.orgSvc != nil {
		if orgsList, err := h.orgSvc.ListOrganizations(sysCtx, nil, nil, 500, 0); err == nil {
			allOrgs = orgsList
			for _, o := range orgsList {
				if o != nil {
					orgNames[o.ID] = o.LegalName
				}
			}
		}
		if branchesList, _, err := h.orgSvc.ListBranchesWithTotal(sysCtx, org.BranchFilter{Status: "active"}, 1000, 0); err == nil {
			allBranches = branchesList
		}
	}

	userNationalIDs := make(map[int64]string)
	userMemberships := make(map[int64]*identity.UserOrgMembership)
	userMFAStatuses := make(map[int64]*identity.MFAStatus)

	if h.idSvc != nil {
		if stats, err := h.idSvc.AdminUserStats(sysCtx); err == nil {
			totalUsers = stats.Total
			staffUsers = stats.Staff
			vendorUsers = stats.Vendor
			customerUsers = stats.Customer
			activeUsers = stats.Active
			suspendedUsers = stats.Suspended
		}

		filter := identity.AdminUserFilter{
			Role:          roleFilter,
			Type:          typeFilter,
			Status:        statusFilter,
			Search:        searchQuery,
			OrgID:         orgIDFilter,
			MFA:           mfaBool,
			LastLoginFrom: loginFromTime,
			LastLoginTo:   loginToTime,
		}
		uList, tot, err := h.idSvc.AdminListUsersWithTotal(sysCtx, filter, limit, offset)
		if err == nil {
			users = uList
			totalCount = tot
		}
		deletionRequests, _ = h.idSvc.AdminListDeletionRequests(sysCtx, "")

		for _, u := range users {
			if u == nil {
				continue
			}
			if nid, err := h.idSvc.GetNationalID(sysCtx, u.ID); err == nil && nid != "" {
				userNationalIDs[u.ID] = nid
			}
			if orgs, err := h.idSvc.ListUserOrganizations(sysCtx, u.ID); err == nil && len(orgs) > 0 {
				userMemberships[u.ID] = orgs[0]
			}
			if st, err := h.idSvc.GetMFAStatus(sysCtx, u.ID); err == nil && st != nil {
				userMFAStatuses[u.ID] = st
			}
		}
	}

	data := pages.AdminUsersPageData{
		Users:            users,
		Organizations:    allOrgs,
		OrgNames:         orgNames,
		AllBranches:      allBranches,
		UserNationalIDs:  userNationalIDs,
		UserMemberships:  userMemberships,
		UserMFAStatuses:  userMFAStatuses,
		DeletionRequests: deletionRequests,
		TotalUsers:       totalUsers,
		CustomerUsers:    customerUsers,
		VendorUsers:      vendorUsers,
		StaffUsers:       staffUsers,
		ActiveUsers:      activeUsers,
		SuspendedUsers:   suspendedUsers,
		Page:             page,
		PerPage:          limit,
		TotalCount:       totalCount,
		SearchQuery:      searchQuery,
		RoleFilter:       roleFilter,
		TypeFilter:       typeFilter,
		StatusFilter:     statusFilter,
		OrgFilter:        orgIDFilter,
		MFAFilter:        mfaFilter,
		LoginFromFilter:  loginFromFilter,
		LoginToFilter:    loginToFilter,
		Notice:           r.URL.Query().Get("notice"),
		NoticeKind:       r.URL.Query().Get("kind"),
	}

	// The create-administrator form appears only for a viewer who may assign
	// roles. Only staff roles are offered: creating an account into a
	// non-staff role produces someone who cannot open the dashboard they were
	// hired for, and the service refuses it anyway.
	if actor := authctx.FromContext(ctx); actor.Can("identity.admin_role.assign") && h.idSvc != nil {
		roles, err := h.idSvc.ListPlatformRoles(ctx)
		if err != nil {
			h.log.ErrorContext(ctx, "list platform roles for the users page", "error", err)
		}
		for _, role := range roles {
			if !role.IsStaff {
				continue
			}
			data.StaffRoles = append(data.StaffRoles, pages.AdminUserRoleOption{
				Key:     role.Key,
				Name:    role.Name.Get(i18n.ParseLang(lang)),
				IsStaff: true,
			})
		}

		// The main moderators a new sub-moderator can be placed under. Only
		// top-level ones are offered: the hierarchy is one level deep, so a
		// moderator who already reports to somebody cannot be a parent, and
		// SetModeratorParent refuses it anyway.
		if mods, err := h.idSvc.ListModerators(sysCtx); err == nil {
			for _, m := range mods {
				if m == nil || !m.IsMain() {
					continue
				}
				data.MainModerators = append(data.MainModerators, pages.AdminModeratorOption{
					UserID: m.UserID,
					Name:   m.Name.Get(i18n.ParseLang(lang)),
					Team:   m.SubordinateCount,
				})
			}
		} else {
			h.log.ErrorContext(ctx, "list moderators for the users page", "error", err)
		}
	}

	h.renderPage(ctx, w, "render admin users page", pages.AdminUsersPage(data, lang, dir))
}
