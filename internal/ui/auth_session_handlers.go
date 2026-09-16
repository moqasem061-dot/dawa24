package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// LogoutSubmit logs the user out, invalidating their session token.
func (h *UIHandler) LogoutSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cookie, err := r.Cookie(h.cookieName())
	if err == nil && cookie.Value != "" && h.idSvc != nil {
		_ = h.idSvc.Logout(ctx, cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secureCookie,
		MaxAge:   -1,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// OrgSwitchSubmit handles context-switching between a user's multiple organizations.
func (h *UIHandler) OrgSwitchSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	targetOrgID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || targetOrgID <= 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	sess, err := h.idSvc.SwitchActiveOrg(ctx, actor.UserID, targetOrgID)
	if err != nil {
		h.log.WarnContext(ctx, "org switch failed", "user_id", actor.UserID, "target_org_id", targetOrgID, "error", err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if sess != nil {
		http.SetCookie(w, &http.Cookie{
			Name:     h.cookieName(),
			Value:    sess.Token,
			Path:     "/",
			HttpOnly: true,
			Secure:   h.secureCookie,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400 * 30,
		})
	}

	http.Redirect(w, r, landingPathForSession(sess), http.StatusSeeOther)
}

// landingPathForSession routes an authenticated session to their home surface.
func landingPathForSession(sess *identity.Session) string {
	if sess == nil {
		return "/catalog"
	}
	if sess.IsStaff() {
		return "/admin/dashboard"
	}
	if sess.Role == identity.RoleJobSeeker && sess.ActiveOrgID == 0 {
		return "/jobs"
	}

	switch sess.OrgStatus {
	case "pending", "under_review":
		return "/onboarding/pending"
	case "rejected":
		return "/onboarding/pending?rejected=1"
	case "suspended":
		return "/onboarding/pending?state=suspended"
	}
	switch sess.OrgType {
	case "vendor":
		return dashboardLanding(rbac.ScopeVendor, sess.Permissions, "/vendor/dashboard")
	case "customer":
		return dashboardLanding(rbac.ScopePharmacy, sess.Permissions, "/customer/dashboard")
	}
	return "/catalog"
}
