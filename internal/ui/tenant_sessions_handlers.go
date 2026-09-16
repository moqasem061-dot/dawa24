package ui

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// TenantSessionsPage renders the dedicated Connected Devices & Active Sessions page
// for Pharmacy customers and Vendors.
func (h *UIHandler) TenantSessionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login?redirect="+r.URL.RequestURI(), http.StatusSeeOther)
		return
	}

	lang, dir := h.localeAndDir(r)
	isVendor := actor.IsVendor()

	// 1. Resolve limits (capped with bounded timeout to never block page rendering)
	maxSessions := 3
	maxDevices := 3
	planName := i18n.T(lang, "tenant.sessions.default_plan")
	if h.idSvc != nil {
		limCtx, limCancel := context.WithTimeout(ctx, 2*time.Second)
		if sMax, dMax, pName, err := h.idSvc.GetOrgPlanLimits(limCtx, actor.OrganizationID); err == nil {
			if sMax > 0 {
				maxSessions = sMax
			}
			if dMax > 0 {
				maxDevices = dMax
			}
			if pName != "" {
				planName = pName
			}
		}
		limCancel()
	}

	// 2. Read live sessions (capped with bounded timeout)
	var currentToken string
	if cookie, err := r.Cookie(h.cookieName()); err == nil && cookie != nil {
		currentToken = cookie.Value
	}

	sessCtx, sessCancel := context.WithTimeout(ctx, 3*time.Second)
	allSessions, _ := h.fetchLiveSessions(sessCtx, actor)
	sessCancel()

	// 3. Current session guarantee: ensure the caller's active device is displayed
	if len(allSessions) == 0 && currentToken != "" && h.idSvc != nil {
		valCtx, valCancel := context.WithTimeout(ctx, 2*time.Second)
		if curSess, err := h.idSvc.ValidateSession(valCtx, currentToken); err == nil && curSess != nil {
			allSessions = []*identity.Session{curSess}
		}
		valCancel()
	}

	viewData := pages.TenantSessionsViewData{
		Sessions:         allSessions,
		CurrentToken:     currentToken,
		CurrentUserID:    actor.UserID,
		IsSuperAdmin:     actor.IsSuperAdmin(),
		MaxLoginSessions: maxSessions,
		MaxDevices:       maxDevices,
		PlanName:         planName,
		IsVendor:         isVendor,
		NoticeType:       r.URL.Query().Get("notice"),
		NoticeMessage:    r.URL.Query().Get("msg"),
	}

	h.renderPage(ctx, w, "render tenant sessions page", pages.TenantSessionsPage(lang, dir, viewData))
}

// TenantSessionRevokeSubmit terminates a specific active session.
func (h *UIHandler) TenantSessionRevokeSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	token := r.PostFormValue("token")
	dest := "/customer/sessions"
	if actor.IsVendor() {
		dest = "/vendor/sessions"
	}

	if token != "" && h.idSvc != nil {
		// Rule: No user can revoke another user's session in the organization unless super_admin
		if targetSess, err := h.idSvc.ValidateSession(ctx, token); err == nil && targetSess != nil {
			if targetSess.UserID != actor.UserID && !actor.IsSuperAdmin() {
				h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "tenant.sessions.super_admin_only_revoke"))
				return
			}
		}

		if actor.OrganizationID > 0 && actor.IsSuperAdmin() {
			_ = h.idSvc.RevokeOrgSession(ctx, actor.OrganizationID, token)
		} else {
			_ = h.idSvc.RevokeSession(ctx, token, actor.UserID)
		}
	}

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "tenant.sessions.revoked_device_success"))
}

// TenantSessionRevokeAllSubmit terminates all sessions except the caller's current session.
func (h *UIHandler) TenantSessionRevokeAllSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	currentToken := r.PostFormValue("current_token")
	if currentToken == "" {
		if cookie, err := r.Cookie(h.cookieName()); err == nil && cookie != nil {
			currentToken = cookie.Value
		}
	}

	dest := "/customer/sessions"
	if actor.IsVendor() {
		dest = "/vendor/sessions"
	}

	if h.idSvc != nil {
		if actor.OrganizationID > 0 && actor.IsSuperAdmin() {
			_ = h.idSvc.RevokeAllOtherOrgSessions(ctx, actor.OrganizationID, currentToken)
		} else {
			_ = h.idSvc.RevokeAllOtherUserSessions(ctx, actor.UserID, currentToken)
		}
	}

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "tenant.sessions.revoked_all_other_success"))
}

// TenantPasswordChangeSubmit updates password from the dedicated sessions & security page.
func (h *UIHandler) TenantPasswordChangeSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := "/customer/sessions"
	if actor.IsVendor() {
		dest = "/vendor/sessions"
	}

	_ = r.ParseForm()
	curr := r.PostFormValue("current_password")
	newPass := r.PostFormValue("new_password")
	confirmPass := r.PostFormValue("confirm_password")

	if strings.TrimSpace(newPass) == "" || newPass != confirmPass {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "settings.password_mismatch"))
		return
	}

	if len(newPass) < 8 {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "tenant.sessions.password_too_short"))
		return
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "tenant.sessions.service_unavailable"))
		return
	}

	if err := h.idSvc.ChangePassword(ctx, actor.UserID, curr, newPass); err != nil {
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "tenant.sessions.password_changed_success"))
}

// Helper to fetch live sessions for the actor/org
func (h *UIHandler) fetchLiveSessions(ctx context.Context, actor authctx.Actor) ([]*identity.Session, error) {
	if h.idSvc == nil {
		return nil, nil
	}
	if actor.OrganizationID > 0 {
		return h.idSvc.ListOrgSessions(ctx, actor.OrganizationID)
	}
	return h.idSvc.ListSessions(ctx, actor.UserID)
}
