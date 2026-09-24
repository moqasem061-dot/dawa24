package ui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/mailer"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

type sendPasswordChangeOTPResp struct {
	OK       bool   `json:"ok"`
	Cooldown int    `json:"cooldown,omitempty"`
	MockCode string `json:"mock_code,omitempty"`
	Error    string `json:"error,omitempty"`
	Message  string `json:"message,omitempty"`
}

// SettingsPasswordSendOTP dispatches a 6-digit OTP to the user's verified email for password change.
func (h *UIHandler) SettingsPasswordSendOTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(sendPasswordChangeOTPResp{
			OK:      false,
			Error:   "unauthorized",
			Message: i18n.T(lang, "common.unauthorized"),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	sysCtx := database.AsSystem(ctx)
	user, err := h.idSvc.GetUserByID(sysCtx, actor.UserID)
	if err != nil || user == nil || user.Email == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(sendPasswordChangeOTPResp{
			OK:      false,
			Error:   "user_not_found",
			Message: i18n.T(lang, "auth.email.invalid_email"),
		})
		return
	}

	engine := h.EmailOTP()
	code, cooldown, err := engine.GenerateOTP(user.Email, "password_change", 15*time.Minute)
	if err != nil {
		if errors.Is(err, mailer.ErrOTPInCooldown) {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(sendPasswordChangeOTPResp{
				OK:       false,
				Error:    "cooldown",
				Cooldown: cooldown,
				Message:  i18n.T(lang, "auth.email.cooldown"),
			})
			return
		}
		if errors.Is(err, mailer.ErrOTPHourlyExceeded) {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(sendPasswordChangeOTPResp{
				OK:      false,
				Error:   "rate_limited",
				Message: i18n.T(lang, "auth.email.rate_limit"),
			})
			return
		}
		h.log.ErrorContext(ctx, "failed to generate password change otp", "user_id", actor.UserID, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(sendPasswordChangeOTPResp{
			OK:      false,
			Error:   "generation_failed",
			Message: i18n.T(lang, "auth.email.error_sending"),
		})
		return
	}

	if sendErr := h.Mailer().SendOTP(ctx, user.Email, code, "password_change", lang); sendErr != nil {
		h.log.ErrorContext(ctx, "failed to send password change otp email", "email", user.Email, "error", sendErr)
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(sendPasswordChangeOTPResp{
			OK:      false,
			Error:   "send_failed",
			Message: i18n.T(lang, "auth.email.error_sending"),
		})
		return
	}

	resp := sendPasswordChangeOTPResp{
		OK:      true,
		Message: i18n.T(lang, "settings.otp_sent_to_email"),
	}
	if h.Mailer().IsMock() && !isProductionEnv() && devShowMockOTP() {
		resp.MockCode = code
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// SettingsPasswordSubmit updates the user's password from the settings form with email OTP verification.
func (h *UIHandler) SettingsPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/settings#security", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	_ = r.ParseForm()
	newPass := strings.TrimSpace(r.PostFormValue("new_password"))
	confirmPass := strings.TrimSpace(r.PostFormValue("new_password_confirmation"))
	otpCode := strings.TrimSpace(r.PostFormValue("email_otp_code"))

	if newPass == "" || newPass != confirmPass {
		h.redirectWithNotice(w, r, "/settings#security", "error", i18n.T(lang, "settings.password_mismatch"))
		return
	}

	if err := identity.ValidatePassword(newPass); err != nil {
		h.redirectWithNotice(w, r, "/settings#security", "error", err.Error())
		return
	}

	sysCtx := database.AsSystem(ctx)
	user, err := h.idSvc.GetUserByID(sysCtx, actor.UserID)
	if err != nil || user == nil {
		h.redirectWithNotice(w, r, "/settings#security", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	// Verify email OTP code
	if otpCode == "" {
		h.redirectWithNotice(w, r, "/settings#security", "error", i18n.T(lang, "settings.otp_required"))
		return
	}

	engine := h.EmailOTP()
	valid, otpErr := engine.VerifyOTP(user.Email, "password_change", otpCode)
	if !valid || otpErr != nil {
		h.redirectWithNotice(w, r, "/settings#security", "error", i18n.T(lang, "settings.invalid_otp"))
		return
	}

	// Update password via system context
	if err := h.idSvc.ResetPassword(sysCtx, user.Email, newPass); err != nil {
		h.log.ErrorContext(ctx, "failed to update user password in settings", "user_id", actor.UserID, "error", err)
		h.redirectWithNotice(w, r, "/settings#security", "error", h.safeMessage(err, lang))
		return
	}

	// Send instant security alert email notifying user of the change
	h.sendPasswordChangeAlertEmail(ctx, user.Email, lang)

	h.log.InfoContext(ctx, "user changed password successfully via settings", "user_id", actor.UserID, "email", user.Email)
	h.redirectWithNotice(w, r, "/settings#security", "success", i18n.T(lang, "settings.password_changed_success"))
}

// sendPasswordChangeAlertEmail sends a security notification via SMTP when a password is changed.
func (h *UIHandler) sendPasswordChangeAlertEmail(ctx context.Context, recipientEmail, lang string) {
	if recipientEmail == "" || h.Mailer() == nil {
		return
	}

	subject, html, text := mailer.RenderPasswordChangedEmail(time.Now(), lang)

	h.safeGo("send-password-change-alert", func() {
		_ = h.Mailer().Send(context.Background(), mailer.Message{
			To:      recipientEmail,
			Subject: subject,
			Text:    text,
			HTML:    html,
		})
	})
}
