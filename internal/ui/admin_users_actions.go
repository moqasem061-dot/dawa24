package ui

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Administrative actions on user accounts.
func (h *UIHandler) adminUserAction(
	w http.ResponseWriter,
	r *http.Request,
	action func(ctx context.Context, userID, actorID int64) error,
) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/users", http.StatusSeeOther)
		return
	}
	if h.idSvc == nil {
		h.renderError(w, r, apperr.Unavailable("identity", nil))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, apperr.Validation("id.invalid", "Invalid user ID", nil))
		return
	}

	// An administrator suspending their own account would lock themselves out
	// of the screen they are standing on, and revoking their own sessions ends
	// the request that did it.
	if id == actor.UserID {
		h.renderError(w, r, apperr.Validation("user.self_action",
			"You cannot apply this action to your own account.", nil))
		return
	}

	redirectTarget := r.PostFormValue("redirect")
	if err := action(ctx, id, actor.UserID); err != nil {
		h.redirectAdminUsers(w, r, redirectTarget, "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectAdminUsers(w, r, redirectTarget, "success", i18n.T(langOf(r), "admin.users.action_success"))
}

func (h *UIHandler) redirectAdminUsers(w http.ResponseWriter, r *http.Request, targetURL, kind, notice string) {
	if targetURL == "" {
		targetURL = r.PostFormValue("redirect")
	}
	if targetURL == "" {
		targetURL = r.URL.Query().Get("redirect")
	}
	if targetURL == "" {
		targetURL = r.Header.Get("Referer")
	}
	if targetURL == "" || !strings.HasPrefix(targetURL, "/admin/users") {
		targetURL = "/admin/users"
	}

	u, err := url.Parse(targetURL)
	if err != nil {
		u = &url.URL{Path: "/admin/users"}
	}
	q := u.Query()
	if notice != "" {
		q.Set("notice", notice)
		q.Set("kind", kind)
	}
	u.RawQuery = q.Encode()

	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}

// AdminUserSuspendSubmit blocks an account and ends its sessions.
func (h *UIHandler) AdminUserSuspendSubmit(w http.ResponseWriter, r *http.Request) {
	h.adminUserAction(w, r, func(ctx context.Context, userID, actorID int64) error {
		return h.idSvc.AdminSuspendUser(ctx, userID, actorID)
	})
}

// AdminUserReactivateSubmit restores a suspended account.
func (h *UIHandler) AdminUserReactivateSubmit(w http.ResponseWriter, r *http.Request) {
	h.adminUserAction(w, r, func(ctx context.Context, userID, actorID int64) error {
		return h.idSvc.AdminReactivateUser(ctx, userID, actorID)
	})
}

// AdminUserResetMFASubmit clears a user's second factor and resets MFA protection.
func (h *UIHandler) AdminUserResetMFASubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/users", http.StatusSeeOther)
		return
	}
	if h.idSvc == nil {
		h.renderError(w, r, apperr.Unavailable("identity", nil))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, apperr.Validation("id.invalid", "Invalid user ID", nil))
		return
	}

	redirectTarget := r.PostFormValue("redirect")
	if err := h.idSvc.AdminResetMFA(ctx, id, actor.UserID); err != nil {
		h.redirectAdminUsers(w, r, redirectTarget, "error", h.safeMessage(err, langOf(r)))
		return
	}

	// Prominent clear notification for admin user management
	successMsg := "تمت إعادة ضبط وتفعيل المصادقة الثنائية (MFA) بنجاح للمستخدم. تم إلغاء الجلسات السابقة وأصبح الحساب جاهزاً لتسجيل الدخول وإعداد الـ MFA من جديد."
	h.redirectAdminUsers(w, r, redirectTarget, "success", successMsg)
}

// AdminUserEditSubmit processes form submission to update user profile, national ID, and org membership.
func (h *UIHandler) AdminUserEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/users", http.StatusSeeOther)
		return
	}
	if h.idSvc == nil {
		h.renderError(w, r, apperr.Unavailable("identity", nil))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.renderError(w, r, apperr.Validation("id.invalid", "Invalid user ID", nil))
		return
	}

	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	email := strings.TrimSpace(r.PostFormValue("email"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	avatarURL := strings.TrimSpace(r.PostFormValue("avatar_url"))
	statusStr := strings.TrimSpace(r.PostFormValue("status"))
	nationalID := strings.TrimSpace(r.PostFormValue("national_id"))
	orgID, _ := strconv.ParseInt(r.PostFormValue("org_id"), 10, 64)
	branchID, _ := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	jobTitle := strings.TrimSpace(r.PostFormValue("job_title"))
	redirectTarget := r.PostFormValue("redirect")

	if email == "" {
		h.redirectAdminUsers(w, r, redirectTarget, "error", i18n.T(langOf(r), "admin.settings.invalid_email"))
		return
	}
	if nameAr == "" && nameEn == "" {
		h.redirectAdminUsers(w, r, redirectTarget, "error", "يجب إدخال اسم المستخدم.")
		return
	}

	// 1. Update identity.users and KYC records
	editIn := identity.AdminEditUserInput{
		NameAr:     nameAr,
		NameEn:     nameEn,
		Email:      email,
		Phone:      phone,
		AvatarURL:  avatarURL,
		Status:     identity.UserStatus(statusStr),
		NationalID: nationalID,
	}

	if err := h.idSvc.AdminUpdateUserDetails(sysCtx, id, editIn, actor.UserID); err != nil {
		h.redirectAdminUsers(w, r, redirectTarget, "error", h.safeMessage(err, langOf(r)))
		return
	}

	// 2. Handle organization membership & branch assignment
	if h.orgSvc != nil && orgID > 0 {
		members, _ := h.orgSvc.ListMembers(sysCtx, orgID)
		var existingMember *org.Member
		for _, m := range members {
			if m != nil && m.UserID == id {
				existingMember = m
				break
			}
		}

		var branchPtr *int64
		if branchID > 0 {
			branchPtr = &branchID
		}

		if existingMember != nil {
			patch := org.MemberPatch{
				BranchID: branchPtr,
				JobTitle: &jobTitle,
			}
			if err := h.orgSvc.UpdateMember(sysCtx, orgID, existingMember.ID, patch); err != nil {
				h.log.ErrorContext(ctx, "update member branch and job title", "error", err)
			}
		} else {
			if newMem, err := h.orgSvc.AddMember(sysCtx, orgID, id, 0); err == nil && newMem != nil {
				patch := org.MemberPatch{
					BranchID: branchPtr,
					JobTitle: &jobTitle,
				}
				_ = h.orgSvc.UpdateMember(sysCtx, orgID, newMem.ID, patch)
				h.notifyUserAddedToOrg(ctx, orgID, id, jobTitle)
			} else if err != nil {
				h.log.ErrorContext(ctx, "add user organization membership", "error", err)
			}
		}
	}

	roleVal := strings.TrimSpace(r.PostFormValue("role"))
	if roleVal == "" && jobTitle != "" {
		roleVal = jobTitle
	}
	if roleVal != "" {
		h.notifyUserRoleChanged(ctx, id, orgID, roleVal)
	}

	if h.resolver != nil {
		h.resolver.Invalidate(id, orgID)
	}

	h.redirectAdminUsers(w, r, redirectTarget, "success", i18n.T(langOf(r), "admin.users.edit_success"))
}

// AdminUserPasswordSubmit sets a new password for a user, revokes active sessions, and notifies the user.
func (h *UIHandler) AdminUserPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/users", http.StatusSeeOther)
		return
	}
	if h.idSvc == nil {
		h.renderError(w, r, apperr.Unavailable("identity", nil))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.renderError(w, r, apperr.Validation("id.invalid", "Invalid user ID", nil))
		return
	}

	password := r.PostFormValue("password")
	confirmPassword := r.PostFormValue("confirm_password")
	redirectTarget := r.PostFormValue("redirect")

	if password == "" {
		h.redirectAdminUsers(w, r, redirectTarget, "error", "يرجى إدخال كلمة المرور الجديدة.")
		return
	}
	if password != confirmPassword {
		h.redirectAdminUsers(w, r, redirectTarget, "error", i18n.T(langOf(r), "admin.users.password_mismatch"))
		return
	}

	if err := h.idSvc.AdminSetPassword(ctx, id, actor.UserID, password); err != nil {
		h.redirectAdminUsers(w, r, redirectTarget, "error", h.safeMessage(err, langOf(r)))
		return
	}

	// Notify user of administrative password reset
	h.notifyUserPasswordSetByAdmin(ctx, id)

	h.redirectAdminUsers(w, r, redirectTarget, "success", i18n.T(langOf(r), "admin.users.password_success"))
}
