package ui

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/mailer"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// ForgotPasswordPage renders the initial step to request a password reset OTP.
func (h *UIHandler) ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	errorMsg := ""
	if r.URL.Query().Get("error") == "invalid_token" {
		errorMsg = i18n.T(lang, "auth.reset.invalid_token")
	}
	email := r.URL.Query().Get("email")
	h.renderPage(ctx, w, "render forgot password page", pages.PasswordReset(errorMsg, "", email, lang, dir))
}

// ForgotSubmit processes the email submitted by the user and sends a 6-digit OTP code.
func (h *UIHandler) ForgotSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	rawEmail := strings.TrimSpace(r.PostFormValue("email"))
	cleanEmail := identity.NormalizeEmail(rawEmail)
	if cleanEmail == "" || !strings.Contains(cleanEmail, "@") || !strings.Contains(cleanEmail, ".") {
		h.renderPage(ctx, w, "render forgot password error", pages.PasswordReset(i18n.T(lang, "auth.email.invalid_email"), "", rawEmail, lang, dir))
		return
	}

	engine := h.EmailOTP()

	// Always look up user in system context
	var userFound bool
	if h.idSvc != nil {
		sysCtx := database.AsSystem(ctx)
		u, err := h.idSvc.GetUserByEmail(sysCtx, cleanEmail)
		if err == nil && u != nil {
			userFound = true
		}
	}

	// If user exists, generate OTP and dispatch email
	if userFound {
		code, _, err := engine.GenerateOTP(cleanEmail, "password_reset", 15*time.Minute)
		if err == nil {
			if sendErr := h.Mailer().SendOTP(ctx, cleanEmail, code, "password_reset", lang); sendErr != nil {
				h.log.WarnContext(ctx, "failed to send password reset otp email", "email", cleanEmail, "error", sendErr)
			}
		} else if errors.Is(err, mailer.ErrOTPInCooldown) {
			http.Redirect(w, r, "/auth/forgot/verify?email="+url.QueryEscape(cleanEmail)+"&error=cooldown", http.StatusSeeOther)
			return
		} else if errors.Is(err, mailer.ErrOTPHourlyExceeded) {
			http.Redirect(w, r, "/auth/forgot/verify?email="+url.QueryEscape(cleanEmail)+"&error=rate_limited", http.StatusSeeOther)
			return
		}
	}

	// For security against user enumeration, redirect to verify step with generic notice
	http.Redirect(w, r, "/auth/forgot/verify?email="+url.QueryEscape(cleanEmail)+"&notice=sent", http.StatusSeeOther)
}

// ForgotVerifyPage renders the 6-digit OTP entry screen for password reset.
func (h *UIHandler) ForgotVerifyPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	email := strings.TrimSpace(r.URL.Query().Get("email"))
	if email == "" {
		http.Redirect(w, r, "/auth/forgot", http.StatusSeeOther)
		return
	}

	var errorMsg string
	switch r.URL.Query().Get("error") {
	case "invalid_code":
		errorMsg = i18n.T(lang, "auth.email.invalid_code")
	case "cooldown":
		errorMsg = i18n.T(lang, "auth.email.cooldown")
	case "rate_limited":
		errorMsg = i18n.T(lang, "auth.email.rate_limit")
	}

	var successMsg string
	if r.URL.Query().Get("notice") == "sent" {
		successMsg = i18n.T(lang, "auth.reset.email_sent")
	}

	h.renderPage(ctx, w, "render forgot verify page", pages.PasswordResetVerify(email, errorMsg, successMsg, lang, dir))
}

// ForgotVerifyOTPSubmit validates the 6-digit OTP code and issues a signed password reset token.
func (h *UIHandler) ForgotVerifyOTPSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	cleanEmail := identity.NormalizeEmail(r.PostFormValue("email"))
	code := strings.TrimSpace(r.PostFormValue("code"))

	if cleanEmail == "" {
		http.Redirect(w, r, "/auth/forgot", http.StatusSeeOther)
		return
	}

	engine := h.EmailOTP()
	valid, err := engine.VerifyOTP(cleanEmail, "password_reset", code)
	if !valid || err != nil {
		errKey := "auth.email.invalid_code"
		if errors.Is(err, mailer.ErrOTPMaxAttempts) {
			errKey = "auth.email.max_attempts"
		} else if errors.Is(err, mailer.ErrOTPExpired) {
			errKey = "auth.email.expired_code"
		}
		h.renderPage(ctx, w, "render verify error", pages.PasswordResetVerify(cleanEmail, i18n.Translate(lang, errKey), "", lang, dir))
		return
	}

	token, err := mailer.SignPasswordResetToken(cleanEmail, h.phoneSecret(), 15*time.Minute)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to sign password reset token", "error", err)
		h.renderPage(ctx, w, "render verify error", pages.PasswordResetVerify(cleanEmail, i18n.T(lang, "auth.reset.invalid_token"), "", lang, dir))
		return
	}

	http.Redirect(w, r, "/auth/reset?token="+url.QueryEscape(token), http.StatusSeeOther)
}

// ResetPasswordPage renders the password creation screen when a valid reset token is provided.
func (h *UIHandler) ResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	token := strings.TrimSpace(r.URL.Query().Get("token"))
	email, err := mailer.VerifyPasswordResetToken(token, h.phoneSecret())
	if err != nil || email == "" {
		http.Redirect(w, r, "/auth/forgot?error=invalid_token", http.StatusSeeOther)
		return
	}

	h.renderPage(ctx, w, "render reset password page", pages.PasswordResetConfirm(token, email, "", lang, dir))
}

// ResetPasswordSubmit receives the new password and persists it to identity.users.
func (h *UIHandler) ResetPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	token := strings.TrimSpace(r.PostFormValue("token"))
	email, err := mailer.VerifyPasswordResetToken(token, h.phoneSecret())
	if err != nil || email == "" {
		http.Redirect(w, r, "/auth/forgot?error=invalid_token", http.StatusSeeOther)
		return
	}

	password := r.PostFormValue("password")
	passwordConfirm := r.PostFormValue("password_confirmation")

	if password == "" || password != passwordConfirm {
		h.renderPage(ctx, w, "render reset password mismatch", pages.PasswordResetConfirm(token, email, i18n.T(lang, "auth.reset.password_mismatch"), lang, dir))
		return
	}

	if err := identity.ValidatePassword(password); err != nil {
		h.renderPage(ctx, w, "render reset password weak", pages.PasswordResetConfirm(token, email, err.Error(), lang, dir))
		return
	}

	if h.idSvc == nil {
		h.renderPage(ctx, w, "render reset service unavailable", pages.PasswordResetConfirm(token, email, i18n.T(lang, "common.service_unavailable"), lang, dir))
		return
	}

	sysCtx := database.AsSystem(ctx)
	if err := h.idSvc.ResetPassword(sysCtx, email, password); err != nil {
		h.log.ErrorContext(ctx, "failed to reset password", "email", email, "error", err)
		h.renderPage(ctx, w, "render reset password error", pages.PasswordResetConfirm(token, email, h.safeMessage(err, lang), lang, dir))
		return
	}

	http.Redirect(w, r, "/auth/login?notice=password_reset_success", http.StatusSeeOther)
}
