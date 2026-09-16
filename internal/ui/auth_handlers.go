package ui

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if actor, ok := authctx.From(ctx); ok && actor.UserID > 0 {
		http.Redirect(w, r, landingPathForActor(actor), http.StatusSeeOther)
		return
	}

	lang, dir := h.localeAndDir(r)
	errorKey := r.URL.Query().Get("error")
	reasonKey := r.URL.Query().Get("reason")
	var errorMsg string
	switch {
	case errorKey == "idle_timeout" || reasonKey == "idle_timeout":
		errorMsg = "تم تسجيل خروجك تلقائياً لعدم وجود نشاط حفاظاً على أمان حسابك. يرجى تسجيل الدخول مجدداً."
	case errorKey == "duplicate_login" || reasonKey == "duplicate_login":
		errorMsg = "تم تسجيل خروجك تلقائياً نظراً لتسجيل الدخول إلى حسابك من جهاز أو متصفح آخر. لا يمكن استخدام الحساب من أكثر من مكان في نفس الوقت."
	case errorKey == "concurrent_limit" || errorKey == "session_evicted" || reasonKey == "concurrent_limit" || reasonKey == "org_concurrent_limit":
		errorMsg = i18n.T(lang, "auth.login.session_evicted")
	case errorKey == "invalid_credentials":
		errorMsg = i18n.T(lang, "auth.login.invalid_credentials")
	case errorKey == "locked":
		errorMsg = i18n.T(lang, "auth.login.account_locked")
	case errorKey == "mfa_unavailable":
		errorMsg = i18n.T(lang, "auth.login.mfa_unavailable")
	case errorKey == "auth_service_unavailable":
		errorMsg = i18n.T(lang, "auth.login.service_unavailable")
	default:
		errorMsg = errorKey
	}

	var successMsg string
	if r.URL.Query().Get("notice") == "password_reset_success" {
		successMsg = i18n.T(lang, "auth.reset.success")
	}

	h.renderPage(ctx, w, "render login page", pages.LoginPage(lang, dir, errorMsg, successMsg))
}

func (h *UIHandler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if actor, ok := authctx.From(ctx); ok && actor.UserID > 0 {
		http.Redirect(w, r, landingPathForActor(actor), http.StatusSeeOther)
		return
	}

	lang, dir := h.localeAndDir(r)

	reqType := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	if reqType == "" {
		reqType = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("role")))
	}
	if reqType == "" {
		reqType = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("account_type")))
	}
	if reqType == "job_seeker" || reqType == "seeker" || reqType == "jobseeker" {
		reqType = "job_seeker"
	} else if reqType == "supplier" || reqType == "vendor" {
		reqType = "vendor"
	} else if reqType == "customer" || reqType == "pharmacy" {
		reqType = "customer"
	} else {
		reqType = ""
	}

	form := pages.RegisterFormData{
		AccountType: reqType,
		Error:       r.URL.Query().Get("error"),
	}

	h.renderPage(ctx, w, "render register page", pages.RegisterPage(lang, dir, form, h.listCities(ctx), h.listGovernorates(ctx)))
}

func (h *UIHandler) OnboardingPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/auth/register", http.StatusFound)
}

func (h *UIHandler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	email := r.PostFormValue("email")
	password := r.PostFormValue("password")
	rememberMe := r.PostFormValue("remember_me")
	redirectURL := r.URL.Query().Get("redirect")

	if h.idSvc == nil {
		http.Redirect(w, r, "/auth/login?error=auth_service_unavailable", http.StatusSeeOther)
		return
	}

	res, err := h.idSvc.Login(ctx, identity.LoginInput{
		Email:     email,
		Password:  password,
		IP:        h.clientIP(r),
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		h.log.WarnContext(ctx, "ui login failed", "email", email, "error", err)
		http.Redirect(w, r, "/auth/login?error=invalid_credentials", http.StatusSeeOther)
		return
	}

	if res.RequiresMFA {
		uid := int64(0)
		uEmail := email
		if res.User != nil {
			uid = res.User.ID
			uEmail = res.User.Email
		}
		p := &pendingMFAPayload{
			UserID:      uid,
			Email:       uEmail,
			OrgID:       0,
			RememberMe:  rememberMe == "1" || rememberMe == "true" || rememberMe == "",
			RedirectURL: redirectURL,
			ExpiresAt:   time.Now().Add(5 * time.Minute).Unix(),
		}
		signedToken := h.signPendingMFAToken(p)
		http.SetCookie(w, &http.Cookie{
			Name:     "dawa24_mfa_pending",
			Value:    signedToken,
			Path:     "/",
			HttpOnly: true,
			Secure:   h.secureCookie,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   300, // 5 minutes
		})
		h.log.InfoContext(ctx, "mfa challenge required for login", "user_id", uid)
		http.Redirect(w, r, "/auth/mfa-verify?redirect="+url.QueryEscape(redirectURL), http.StatusSeeOther)
		return
	}

	maxAge := 86400 * 30 // Default remember-me: 30 days
	if rememberMe == "0" || rememberMe == "false" {
		maxAge = 86400 // 1 day session
	}

	if res.Session != nil {
		http.SetCookie(w, &http.Cookie{
			Name:     h.cookieName(),
			Value:    res.Session.Token,
			Path:     "/",
			HttpOnly: true,
			Secure:   h.secureCookie,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   maxAge,
		})

		if res.Session.ActiveOrgID > 0 {
			orgID := res.Session.ActiveOrgID
			h.safeGo("ensure-org-ai-gateway-provisioned", func() {
				h.EnsureOrgAIGatewayProvisioned(context.Background(), orgID)
			})
		}
	}

	// An explicit redirect (from a protected page the user was heading to)
	// wins; otherwise route by session — account type, approval state and
	// platform role decide where the user lands.
	redirectURL = safeLocalRedirect(redirectURL, "")
	if redirectURL == "" {
		redirectURL = landingPathForSession(res.Session)
	}

	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// MFAVerifyPage renders the 6-digit TOTP challenge screen during login.
func (h *UIHandler) MFAVerifyPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	redirectURL := r.URL.Query().Get("redirect")
	errorKey := r.URL.Query().Get("error")

	cookie, err := r.Cookie("dawa24_mfa_pending")
	if err != nil || cookie == nil || cookie.Value == "" {
		http.Redirect(w, r, "/auth/login?error=mfa_session_expired", http.StatusSeeOther)
		return
	}

	payload, err := h.parsePendingMFAToken(cookie.Value)
	if err != nil || payload == nil || time.Now().Unix() > payload.ExpiresAt {
		http.Redirect(w, r, "/auth/login?error=mfa_session_expired", http.StatusSeeOther)
		return
	}

	var errorMsg string
	if errorKey == "invalid_code" {
		errorMsg = i18n.T(lang, "mfa.code_invalid_or_expired")
	} else if errorKey != "" {
		errorMsg = errorKey
	}

	h.renderPage(ctx, w, "render mfa verify page", pages.MFAVerifyPage(lang, dir, payload.Email, errorMsg, redirectURL))
}

// MFAVerifySubmit processes the 6-digit TOTP or backup code and issues the full session cookie.
func (h *UIHandler) MFAVerifySubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cookie, err := r.Cookie("dawa24_mfa_pending")
	if err != nil || cookie == nil || cookie.Value == "" {
		http.Redirect(w, r, "/auth/login?error=mfa_session_expired", http.StatusSeeOther)
		return
	}

	payload, err := h.parsePendingMFAToken(cookie.Value)
	if err != nil || payload == nil || time.Now().Unix() > payload.ExpiresAt {
		http.Redirect(w, r, "/auth/login?error=mfa_session_expired", http.StatusSeeOther)
		return
	}

	code := strings.TrimSpace(r.PostFormValue("code"))
	recoveryCode := strings.TrimSpace(r.PostFormValue("recovery_code"))
	targetCode := code
	if targetCode == "" {
		targetCode = recoveryCode
	}

	redirectURL := r.PostFormValue("redirect")
	if redirectURL == "" {
		redirectURL = payload.RedirectURL
	}

	if targetCode == "" {
		http.Redirect(w, r, "/auth/mfa-verify?error=invalid_code&redirect="+url.QueryEscape(redirectURL), http.StatusSeeOther)
		return
	}

	if h.idSvc == nil {
		http.Redirect(w, r, "/auth/login?error=auth_service_unavailable", http.StatusSeeOther)
		return
	}

	valid, err := h.idSvc.VerifyMFA(ctx, payload.UserID, targetCode)
	if err != nil || !valid {
		h.log.WarnContext(ctx, "mfa verification failed", "user_id", payload.UserID, "error", err)
		http.Redirect(w, r, "/auth/mfa-verify?error=invalid_code&redirect="+url.QueryEscape(redirectURL), http.StatusSeeOther)
		return
	}

	// MFA verified! Clear pending cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "dawa24_mfa_pending",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	sess, err := h.idSvc.CompleteMFALogin(ctx, payload.UserID, payload.OrgID, h.clientIP(r), r.UserAgent())
	if err != nil {
		h.log.ErrorContext(ctx, "complete mfa login error", "user_id", payload.UserID, "error", err)
		http.Redirect(w, r, "/auth/login?error=auth_service_unavailable", http.StatusSeeOther)
		return
	}

	maxAge := 86400 * 30
	if !payload.RememberMe {
		maxAge = 86400
	}

	http.SetCookie(w, &http.Cookie{
		Name:     h.cookieName(),
		Value:    sess.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})

	if sess.ActiveOrgID > 0 {
		h.safeGo("ensure-org-ai-gateway-provisioned", func() {
			h.EnsureOrgAIGatewayProvisioned(context.Background(), sess.ActiveOrgID)
		})
	}

	redirectURL = safeLocalRedirect(redirectURL, "")
	if redirectURL == "" {
		redirectURL = landingPathForSession(sess)
	}

	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

type pendingMFAPayload struct {
	UserID      int64  `json:"uid"`
	Email       string `json:"em"`
	OrgID       int64  `json:"oid"`
	RememberMe  bool   `json:"rm"`
	RedirectURL string `json:"rd,omitempty"`
	ExpiresAt   int64  `json:"exp"`
}

func (h *UIHandler) signPendingMFAToken(p *pendingMFAPayload) string {
	b, _ := json.Marshal(p)
	payloadB64 := base64.RawURLEncoding.EncodeToString(b)
	sigKey := []byte("dawa24_mfa_intermediate_key_secret_2026")
	mac := hmac.New(sha256.New, sigKey)
	mac.Write([]byte(payloadB64))
	sig := hex.EncodeToString(mac.Sum(nil))
	return payloadB64 + "." + sig
}

func (h *UIHandler) parsePendingMFAToken(tokenStr string) (*pendingMFAPayload, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 2 {
		return nil, identity.ErrInvalidCredentials
	}
	payloadB64, sig := parts[0], parts[1]
	sigKey := []byte("dawa24_mfa_intermediate_key_secret_2026")
	mac := hmac.New(sha256.New, sigKey)
	mac.Write([]byte(payloadB64))
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	if hmac.Equal([]byte(sig), []byte(expectedSig)) == false {
		return nil, identity.ErrInvalidCredentials
	}

	b, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, err
	}
	var p pendingMFAPayload
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// landingPathForActor routes an authenticated actor to their home surface.
func landingPathForActor(actor authctx.Actor) string {
	if actor.IsStaff {
		return "/admin/dashboard"
	}
	if actor.IsJobSeeker() {
		return "/jobs"
	}
	switch actor.OrgStatus {
	case "pending", "under_review":
		return "/onboarding/pending"
	case "rejected":
		return "/onboarding/pending?rejected=1"
	case "suspended":
		return "/onboarding/pending?state=suspended"
	}
	switch actor.OrgType {
	case "vendor":
		return dashboardLanding(rbac.ScopeVendor, actor.Permissions, "/vendor/dashboard")
	case "customer":
		return dashboardLanding(rbac.ScopePharmacy, actor.Permissions, "/customer/dashboard")
	}
	return "/catalog"
}
