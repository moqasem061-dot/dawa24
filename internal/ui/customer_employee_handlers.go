package ui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// The pharmacy's employee writes.
//
// Two contracts are fixed here, and both were bugs anyone could hit.
//
// **{id} is the org.members row, everywhere.** It used to be the user id on
// /edit and /delete and the member id on /status, and the two templates picked
// whichever suited the row they were rendering — so the team page's delete
// button posted a member id to a handler that read it as a user id and removed
// somebody else, or nobody. Every route below resolves the member first.
//
// **An edit writes only what the form carried.** Editing went through
// AddMemberDirect, an upsert built from a freshly-constructed org.Member, so a
// form that changed a job title also cleared the branch, the employee code and
// the role, because those arrived as zero values.

// teamBack is where an employee action returns to.
const teamBack = "/customer/team"

// CustomerEmployeeCreateSubmit creates an employee account and binds it to the
// branch and role chosen.
func (h *UIHandler) CustomerEmployeeCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if !ok || orgID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+teamBack, http.StatusSeeOther)
		return
	}
	if h.idSvc == nil || h.orgSvc == nil {
		h.redirectWithNotice(w, r, teamBack, "error", i18n.T(lang, "common.service_unavailable"))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, teamBack, "error", i18n.T(lang, "common.invalid_form_data"))
		return
	}

	email := strings.TrimSpace(r.PostFormValue("email"))
	name := strings.TrimSpace(r.PostFormValue("name"))
	if email == "" || name == "" {
		h.redirectWithNotice(w, r, teamBack, "error", i18n.T(lang, "customer.employee.name_email_required"))
		return
	}

	sysCtx := database.AsSystem(ctx)
	targetUserID, err := h.resolveEmployeeUser(sysCtx, orgID, email, name,
		strings.TrimSpace(r.PostFormValue("phone")),
		strings.TrimSpace(r.PostFormValue("password")), lang)
	if err != nil {
		h.redirectWithNotice(w, r, teamBack, "error", err.Error())
		return
	}

	roleID, roleKey := h.resolveTeamRole(ctx, orgID, r.PostFormValue("role_id"), r.PostFormValue("role_key"))
	branchID := h.resolveTeamBranch(ctx, orgID, r.PostFormValue("branch_id"))

	employeeCode := strings.TrimSpace(r.PostFormValue("employee_code"))
	if employeeCode == "" {
		employeeCode = fmt.Sprintf("EMP-%d", targetUserID)
	}

	member := &org.Member{
		OrganizationID: orgID,
		UserID:         targetUserID,
		BranchID:       branchID,
		RoleID:         roleID,
		OrgRoleID:      nonZero(roleID),
		RoleKey:        roleKey,
		EmployeeCode:   employeeCode,
		JobTitle:       strings.TrimSpace(r.PostFormValue("job_title")),
		IsActive:       true,
	}
	if err := h.orgSvc.AddMemberDirect(sysCtx, member); err != nil {
		h.redirectWithNotice(w, r, teamBack, "error", h.safeMessage(err, lang))
		return
	}
	if roleKey == "org_manager" && branchID != nil {
		if err := h.orgSvc.AssignBranchManager(sysCtx, orgID, *branchID, &targetUserID); err != nil {
			h.log.ErrorContext(ctx, "assign branch manager", "error", err,
				"organization_id", orgID, "branch_id", *branchID)
		}
	}

	h.invalidatePermissions(targetUserID, orgID)
	h.redirectWithNotice(w, r, teamBack, "success", i18n.T(lang, "customer.employee.create_success"))
}

// CustomerEmployeeEditSubmit updates only the fields the form carried.
func (h *UIHandler) CustomerEmployeeEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, orgID, ok := h.teamActor(r)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect="+teamBack, http.StatusSeeOther)
		return
	}
	_ = actor

	member, ok := h.loadTeamMember(w, r, orgID, lang)
	if !ok {
		return
	}
	if member.RoleKey == "org_owner" {
		h.redirectWithNotice(w, r, teamBack, "error", "لا يمكن تعديل بيانات مالك المنشأة من قائمة الموظفين")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, teamBack, "error", i18n.T(lang, "common.invalid_form_data"))
		return
	}

	patch := org.MemberPatch{}
	if r.PostForm.Has("job_title") {
		v := strings.TrimSpace(r.PostFormValue("job_title"))
		patch.JobTitle = &v
	}
	if r.PostForm.Has("employee_code") {
		v := strings.TrimSpace(r.PostFormValue("employee_code"))
		patch.EmployeeCode = &v
	}
	if r.PostForm.Has("branch_id") {
		patch.BranchID = h.resolveTeamBranch(ctx, orgID, r.PostFormValue("branch_id"))
		if patch.BranchID == nil {
			zero := int64(0)
			patch.BranchID = &zero
		}
	}
	if r.PostForm.Has("role_id") || r.PostForm.Has("role_key") {
		roleID, roleKey := h.resolveTeamRole(ctx, orgID, r.PostFormValue("role_id"), r.PostFormValue("role_key"))
		patch.OrgRoleID = nonZero(roleID)
		patch.RoleKey = &roleKey
	}
	// A checkbox that is off is not submitted at all, so its absence is the
	// answer rather than a missing field — but only when the form was the edit
	// dialog, which always carries the marker below.
	if r.PostForm.Has("is_active") || r.PostForm.Has("job_title") {
		active := isTruthy(r.PostFormValue("is_active"))
		patch.IsActive = &active
	}

	if err := h.orgSvc.UpdateMember(database.AsSystem(ctx), orgID, member.ID, patch); err != nil {
		h.redirectWithNotice(w, r, teamBack, "error", h.safeMessage(err, lang))
		return
	}
	if patch.RoleKey != nil && *patch.RoleKey == "org_manager" && patch.BranchID != nil && *patch.BranchID > 0 {
		if err := h.orgSvc.AssignBranchManager(database.AsSystem(ctx), orgID, *patch.BranchID, &member.UserID); err != nil {
			h.log.ErrorContext(ctx, "assign branch manager", "error", err,
				"organization_id", orgID, "branch_id", *patch.BranchID)
		}
	}

	h.invalidatePermissions(member.UserID, orgID)
	h.redirectWithNotice(w, r, teamBack, "success", i18n.T(lang, "customer.employee.update_success"))
}

// CustomerEmployeeDeleteSubmit removes a membership.
func (h *UIHandler) CustomerEmployeeDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, orgID, ok := h.teamActor(r)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect="+teamBack, http.StatusSeeOther)
		return
	}

	member, ok := h.loadTeamMember(w, r, orgID, lang)
	if !ok {
		return
	}
	if member.UserID == actor.UserID {
		h.redirectWithNotice(w, r, teamBack, "error", i18n.T(lang, "customer.employee.cannot_remove_self"))
		return
	}
	if member.RoleKey == "org_owner" {
		h.redirectWithNotice(w, r, teamBack, "error", "لا يمكن حذف مالك المنشأة من قائمة الموظفين")
		return
	}

	if err := h.orgSvc.RemoveMember(database.AsSystem(ctx), orgID, member.UserID); err != nil {
		h.redirectWithNotice(w, r, teamBack, "error", h.safeMessage(err, lang))
		return
	}
	h.invalidatePermissions(member.UserID, orgID)
	h.redirectWithNotice(w, r, teamBack, "success", i18n.T(lang, "customer.employee.delete_success"))
}

// CustomerEmployeeStatusSubmit toggles a membership's active flag.
func (h *UIHandler) CustomerEmployeeStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, orgID, ok := h.teamActor(r)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect="+teamBack, http.StatusSeeOther)
		return
	}

	member, ok := h.loadTeamMember(w, r, orgID, lang)
	if !ok {
		return
	}
	if member.UserID == actor.UserID {
		h.redirectWithNotice(w, r, teamBack, "error", "لا يمكنك تغيير حالة تفعيل حسابك الخاص.")
		return
	}
	if member.RoleKey == "org_owner" {
		h.redirectWithNotice(w, r, teamBack, "error", "لا يمكن تعديل حالة تفعيل مالك المنشأة")
		return
	}
	if err := h.orgSvc.ToggleMemberStatus(database.AsSystem(ctx), orgID, member.ID); err != nil {
		h.redirectWithNotice(w, r, teamBack, "error", h.safeMessage(err, lang))
		return
	}
	h.invalidatePermissions(member.UserID, orgID)
	h.redirectWithNotice(w, r, teamBack, "success", i18n.T(lang, "customer.employee.status_success"))
}

// teamActor resolves the acting member and their organization.
func (h *UIHandler) teamActor(r *http.Request) (authctx.Actor, int64, bool) {
	actor, ok := authctx.From(r.Context())
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if !ok || orgID <= 0 || h.orgSvc == nil {
		return actor, 0, false
	}
	return actor, orgID, true
}

// loadTeamMember reads {id} as an org.members row of the caller's own company.
//
// Scoping the read by organization is what stops one company addressing
// another's staff by changing a number in the URL.
func (h *UIHandler) loadTeamMember(
	w http.ResponseWriter, r *http.Request, orgID int64, lang string,
) (*org.Member, bool) {
	memberID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || memberID <= 0 {
		h.redirectWithNotice(w, r, teamBack, "error", i18n.T(lang, "customer.employee.invalid_id"))
		return nil, false
	}
	member, err := h.orgSvc.GetMember(r.Context(), orgID, memberID)
	if err != nil || member == nil {
		h.redirectWithNotice(w, r, teamBack, "error", i18n.T(lang, "customer.employee.invalid_id"))
		return nil, false
	}
	return member, true
}

// resolveEmployeeUser finds or creates the account an employee row points at.
func (h *UIHandler) resolveEmployeeUser(
	sysCtx context.Context, orgID int64, email, name, phone, password, lang string,
) (int64, error) {
	if existing, err := h.idSvc.GetUserByEmail(sysCtx, email); err == nil && existing != nil {
		// Reusing an account is right for someone joining their own pharmacy
		// and wrong for an address that already works for another company:
		// AddMember would quietly hand this company a membership of them.
		elsewhere, checkErr := h.orgSvc.UserBelongsElsewhere(sysCtx, existing.ID, orgID)
		if checkErr != nil {
			return 0, fmt.Errorf("%s", i18n.T(lang, "common.service_unavailable"))
		}
		if elsewhere {
			return 0, fmt.Errorf("%s", i18n.T(lang, "customer.employee.belongs_to_other_org"))
		}
		return existing.ID, nil
	}

	if password == "" {
		randBytes := make([]byte, 8)
		if _, err := rand.Read(randBytes); err != nil {
			return 0, fmt.Errorf("%s", i18n.T(lang, "common.service_unavailable"))
		}
		password = fmt.Sprintf("Dawa24!%s", hex.EncodeToString(randBytes))
	}
	newUser, _, err := h.idSvc.Register(sysCtx, identity.RegisterInput{
		Email: email, Password: password, NameAr: name, NameEn: name,
		Phone: phone, Role: "user",
	})
	if err != nil {
		return 0, fmt.Errorf("%s", h.safeMessage(err, lang))
	}
	return newUser.ID, nil
}

func nonZero(v int64) *int64 {
	if v <= 0 {
		return nil
	}
	return &v
}

func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "on", "1", "yes":
		return true
	default:
		return false
	}
}
