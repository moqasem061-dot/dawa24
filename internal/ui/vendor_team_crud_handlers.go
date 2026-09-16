package ui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// VendorTeamNewSubmit registers an employee and links them to the vendor org.
func (h *UIHandler) VendorTeamNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/team", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}

	name := strings.TrimSpace(r.PostFormValue("name"))
	email := strings.ToLower(strings.TrimSpace(r.PostFormValue("email")))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	password := strings.TrimSpace(r.PostFormValue("password"))
	roleID, roleKey := h.resolveTeamRole(ctx, actor.OrganizationID, r.PostFormValue("role_id"), r.PostFormValue("role_key"))
	jobTitle := strings.TrimSpace(r.PostFormValue("job_title"))
	employeeCode := strings.TrimSpace(r.PostFormValue("employee_code"))

	if name == "" {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.enter_full_name"))
		return
	}
	if email == "" || !strings.Contains(email, "@") {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.enter_valid_email"))
		return
	}
	if roleKey == "" {
		roleKey = "org_employee"
	}
	if password == "" || len(password) < 6 {
		randomBytes := make([]byte, 16)
		if _, err := rand.Read(randomBytes); err == nil {
			password = hex.EncodeToString(randomBytes) + "!A1"
		} else {
			password = "Dw24_" + strconv.FormatInt(time.Now().UnixNano(), 36) + "!A1"
		}
	}

	var branchID *int64
	if bStr := strings.TrimSpace(r.PostFormValue("branch_id")); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			branchID = &bID
		}
	}

	if h.idSvc == nil || h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "common.service_unavailable"))
		return
	}

	// 1. Locate existing user account or create a new one
	var targetUserID int64
	existingUser, err := h.idSvc.GetUserByEmail(ctx, email)
	if err == nil && existingUser != nil {
		targetUserID = existingUser.ID
	} else {
		newUser, _, regErr := h.idSvc.Register(ctx, identity.RegisterInput{
			Email:    email,
			Password: password,
			NameAr:   name,
			NameEn:   name,
			Phone:    phone,
			Role:     "user",
		})
		if regErr != nil {
			h.log.ErrorContext(ctx, "failed to register employee user", "email", email, "error", regErr)
			h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.register_failed_prefix")+h.safeMessage(regErr, langOf(r)))
			return
		}
		targetUserID = newUser.ID
	}

	if employeeCode == "" {
		employeeCode = fmt.Sprintf("EMP-%d", targetUserID)
	}

	// 2. Link member to vendor organization with all specified attributes
	member := &org.Member{
		OrganizationID: actor.OrganizationID,
		UserID:         targetUserID,
		BranchID:       branchID,
		RoleID:         roleID,
		OrgRoleID:      nonZero(roleID),
		RoleKey:        roleKey,
		JobTitle:       jobTitle,
		EmployeeCode:   employeeCode,
		IsActive:       true,
	}

	sysCtx := database.AsSystem(ctx)
	if err := h.orgSvc.AddMemberDirect(sysCtx, member); err != nil {
		h.log.ErrorContext(ctx, "failed to add org member", "error", err, "org_id", actor.OrganizationID, "user_id", targetUserID)
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.link_failed_prefix")+err.Error())
		return
	}

	if roleID > 0 {
		if err := h.orgSvc.AssignMemberRole(sysCtx, actor.OrganizationID, member.ID, roleID); err != nil {
			h.log.WarnContext(ctx, "vendor employee new: assign role warning", "member_id", member.ID, "role_id", roleID, "error", err)
		}
	}

	if roleKey == "org_manager" && branchID != nil {
		_ = h.orgSvc.AssignBranchManager(sysCtx, actor.OrganizationID, *branchID, &targetUserID)
	}

	h.invalidatePermissions(targetUserID, actor.OrganizationID)
	h.invalidatePermissions(actor.UserID, actor.OrganizationID)

	h.redirectWithNotice(w, r, "/vendor/team", "success", fmt.Sprintf(i18n.T(langOf(r), "vendor.team.employee_added_success"), name))
}

// VendorTeamEditSubmit updates an existing employee's profile (name, phone) and
// membership (job title, employee code, role, branch, active flag). The vendor
// team screen addresses employees by membership id, so this resolves the
// underlying user through GetMemberByID before touching the identity record.
func (h *UIHandler) VendorTeamEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/team", http.StatusSeeOther)
		return
	}

	memberID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || memberID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.invalid_employee_id"))
		return
	}

	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "common.org_service_unavailable"))
		return
	}

	member, err := h.orgSvc.GetMemberByID(ctx, actor.OrganizationID, memberID)
	if err != nil || member == nil || member.UserID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.invalid_employee_id"))
		return
	}
	if member.RoleKey == "org_owner" {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.cannot_edit_owner"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}

	name := strings.TrimSpace(r.PostFormValue("name"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	jobTitle := strings.TrimSpace(r.PostFormValue("job_title"))
	employeeCode := strings.TrimSpace(r.PostFormValue("employee_code"))
	roleID, roleKey := h.resolveTeamRole(ctx, actor.OrganizationID, r.PostFormValue("role_id"), r.PostFormValue("role_key"))
	if roleKey == "" {
		roleKey = member.RoleKey
	}
	if roleKey == "" {
		roleKey = "org_employee"
	}
	if roleID == 0 && member.OrgRoleID != nil {
		roleID = *member.OrgRoleID
	}
	if roleID == 0 && member.RoleID > 0 {
		roleID = member.RoleID
	}
	isActive := r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on" || r.PostFormValue("is_active") == "1"

	var branchID *int64
	if bStr := strings.TrimSpace(r.PostFormValue("branch_id")); bStr != "" {
		if bID, e := strconv.ParseInt(bStr, 10, 64); e == nil && bID > 0 {
			branchID = &bID
		}
	}

	sysCtx := database.AsSystem(ctx)

	// 1. Profile fields on the identity user. UpdateProfile ignores empty
	//    values, so a blank field leaves the stored one untouched.
	if h.idSvc != nil && (name != "" || phone != "") {
		if _, e := h.idSvc.UpdateProfile(sysCtx, member.UserID, name, name, phone, "", ""); e != nil {
			h.log.WarnContext(ctx, "vendor employee edit: profile update failed", "user_id", member.UserID, "error", e)
		}
	}

	// 2. Membership fields (AddMember upserts on organization_id + user_id).
	updated := &org.Member{
		ID:             member.ID,
		OrganizationID: actor.OrganizationID,
		UserID:         member.UserID,
		BranchID:       branchID,
		RoleKey:        roleKey,
		OrgRoleID:      nonZero(roleID),
		RoleID:         roleID,
		JobTitle:       jobTitle,
		EmployeeCode:   employeeCode,
		IsActive:       isActive,
	}
	if err := h.orgSvc.AddMemberDirect(sysCtx, updated); err != nil {
		h.log.ErrorContext(ctx, "vendor employee edit: member update failed", "member_id", memberID, "error", err)
		h.redirectWithNotice(w, r, "/vendor/team", "error", h.safeMessage(err, langOf(r)))
		return
	}

	if roleID > 0 {
		if err := h.orgSvc.AssignMemberRole(sysCtx, actor.OrganizationID, memberID, roleID); err != nil {
			h.log.WarnContext(ctx, "vendor employee edit: assign role warning", "member_id", memberID, "role_id", roleID, "error", err)
		}
	}

	if roleKey == "org_manager" && branchID != nil {
		_ = h.orgSvc.AssignBranchManager(sysCtx, actor.OrganizationID, *branchID, &member.UserID)
	}

	h.invalidatePermissions(member.UserID, actor.OrganizationID)
	h.invalidatePermissions(actor.UserID, actor.OrganizationID)

	h.redirectWithNotice(w, r, "/vendor/team", "success", i18n.T(langOf(r), "vendor.team.employee_updated_success"))
}

// VendorTeamToggleSubmit toggles a member's active status.
func (h *UIHandler) VendorTeamToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.invalid_employee_id"))
		return
	}
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "common.org_service_unavailable"))
		return
	}
	member, err := h.orgSvc.GetMemberByID(ctx, actor.OrganizationID, id)
	if err != nil || member == nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.invalid_employee_id"))
		return
	}
	if member.UserID == actor.UserID {
		h.redirectWithNotice(w, r, "/vendor/team", "error", "لا يمكنك تغيير حالة تفعيل حسابك الخاص")
		return
	}
	if member.RoleKey == "org_owner" {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.cannot_edit_owner"))
		return
	}
	if err := h.orgSvc.ToggleMemberStatus(ctx, actor.OrganizationID, id); err != nil {
		h.log.ErrorContext(ctx, "toggle member status", "error", err, "member", id, "org", actor.OrganizationID)
		h.redirectWithNotice(w, r, "/vendor/team", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/team", "success", i18n.T(langOf(r), "vendor.team.status_updated_success"))
}

// VendorTeamDeleteSubmit removes an employee member from the organization.
func (h *UIHandler) VendorTeamDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.invalid_employee_id"))
		return
	}
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "common.org_service_unavailable"))
		return
	}
	// The row is addressed by membership id; RemoveMember deletes by user id.
	member, err := h.orgSvc.GetMemberByID(ctx, actor.OrganizationID, id)
	if err != nil || member == nil || member.UserID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.invalid_employee_id"))
		return
	}
	if member.UserID == actor.UserID {
		h.redirectWithNotice(w, r, "/vendor/team", "error", "لا يمكنك حذف حسابك الخاص من المنشأة")
		return
	}
	if member.RoleKey == "org_owner" {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.cannot_edit_owner"))
		return
	}
	if err := h.orgSvc.RemoveMember(ctx, actor.OrganizationID, member.UserID); err != nil {
		h.log.ErrorContext(ctx, "remove member", "error", err, "member", id, "org", actor.OrganizationID)
		h.redirectWithNotice(w, r, "/vendor/team", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/team", "success", i18n.T(langOf(r), "vendor.team.deleted_success"))
}

// VendorTeamImportUploadSubmit handles file upload and creates new employee import session for vendor.
func (h *UIHandler) VendorTeamImportUploadSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportUploadSubmit(w, r, "vendor")
}

// VendorTeamImportMapSubmit saves user column mappings and role mappings, then builds review rows.
func (h *UIHandler) VendorTeamImportMapSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportMapSubmit(w, r, "vendor")
}

// VendorTeamImportCommitSubmit creates user accounts and links employees to vendor org.
func (h *UIHandler) VendorTeamImportCommitSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportCommitSubmit(w, r, "vendor", "org_employee")
}

// VendorTeamImportCancelSubmit cancels and deletes an import session.
func (h *UIHandler) VendorTeamImportCancelSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportCancelSubmit(w, r, "vendor")
}
