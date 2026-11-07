package ui

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/mailer"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// ForgotPasswordPage renders the initial step to request a password reset OTP.
func (h *UIHandler) ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	errorMsg := ""
	switch r.URL.Query().Get("error") {
	case "invalid_token":
		errorMsg = i18n.T(lang, "auth.reset.invalid_token")
	case "send_failed":
		errorMsg = i18n.T(lang, "auth.email.error_sending")
	case "user_not_found":
		errorMsg = "البريد الإلكتروني المدخل غير مسجل لدينا، يرجى التأكد من كتابة البريد أو إنشاء حساب جديد"
	case "cooldown":
		errorMsg = i18n.T(lang, "auth.email.cooldown")
	case "rate_limited":
		errorMsg = i18n.T(lang, "auth.email.rate_limit")
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

	// The answer is the same whether or not the email is registered: the code is
	// sent only to a registered account, and every outcome lands on the verify
	// page with the same notice. Failures are logged, not shown, so the screen
	// cannot be used to learn which emails have accounts.
	if h.idSvc != nil {
		sysCtx := database.AsSystem(ctx)
		if u, err := h.idSvc.GetUserByEmail(sysCtx, cleanEmail); err == nil && u != nil {
			h.sendPasswordResetCode(ctx, cleanEmail, lang)
		}
	}

	http.Redirect(w, r, "/auth/forgot/verify?email="+url.QueryEscape(cleanEmail)+"&notice=sent", http.StatusSeeOther)
}

// sendPasswordResetCode generates the reset code and emails it. Cooldown,
// hourly-limit and send failures are logged only; the caller shows one message.
func (h *UIHandler) sendPasswordResetCode(ctx context.Context, cleanEmail, lang string) {
	engine := h.EmailOTP()
	code, _, err := engine.GenerateOTP(cleanEmail, "password_reset", 15*time.Minute)
	if err != nil {
		h.log.InfoContext(ctx, "password reset code not sent", "error", err)
		return
	}
	if sendErr := h.Mailer().SendOTP(ctx, cleanEmail, code, "password_reset", lang); sendErr != nil {
		h.log.ErrorContext(ctx, "failed to send password reset otp email", "email", cleanEmail, "error", sendErr)
	}
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
	case "send_failed":
		errorMsg = i18n.T(lang, "auth.email.error_sending")
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
	email, _, err := mailer.VerifyPasswordResetToken(token, h.phoneSecret())
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
	email, issuedAt, err := mailer.VerifyPasswordResetToken(token, h.phoneSecret())
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
	if err := h.idSvc.ResetPasswordWithToken(sysCtx, email, password, issuedAt); err != nil {
		if errors.Is(err, identity.ErrResetTokenSpent) {
			http.Redirect(w, r, "/auth/forgot?error=invalid_token", http.StatusSeeOther)
			return
		}
		h.log.ErrorContext(ctx, "failed to reset password", "email", email, "error", err)
		h.renderPage(ctx, w, "render reset password error", pages.PasswordResetConfirm(token, email, h.safeMessage(err, lang), lang, dir))
		return
	}

	http.Redirect(w, r, "/auth/login?notice=password_reset_success", http.StatusSeeOther)
}
