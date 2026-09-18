package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/mailer"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type sendPasswordChangeOTPResp struct {
	OK        bool   `json:"ok"`
	Cooldown  int    `json:"cooldown,omitempty"`
	MockCode  string `json:"mock_code,omitempty"`
	Error     string `json:"error,omitempty"`
	Message   string `json:"message,omitempty"`
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

	subject := "تنبيه أمان: تم تغيير كلمة المرور لحسابك في دوا 24 | Security Alert"
	if lang == "en" {
		subject = "Security Alert: Password Changed for your Dawa24 account"
	}

	nowStr := time.Now().Format("2006-01-02 15:04:05 MST")
	text := fmt.Sprintf(
		"مرحباً،\n\nنود إعلامك بأنه تم تغيير كلمة المرور الخاصة بحسابك في منصة دوا 24 بنجاح في: %s.\n\nإذا قمت أنت بهذا الإجراء، فلا داعي لاتخاذ أي خطوة.\n\nإذا لم تكن أنت من قام بتغيير كلمة المرور، يرجى التوجه فوراً لرابط استعادة كلمة المرور أو التواصل مع إدارة المنصة لحماية حسابك.\n\nفريق أمان دوا 24\nsupport@dawa24.com",
		nowStr,
	)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html dir="rtl" lang="ar">
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background-color: #f1f5f9; padding: 24px 0; margin: 0;">
  <table width="100%%" border="0" cellspacing="0" cellpadding="0">
    <tr><td align="center">
      <table width="600" style="background: #ffffff; border-radius: 12px; overflow: hidden; box-shadow: 0 4px 6px rgba(0,0,0,0.05);">
        <tr>
          <td style="background: #1e3a8a; padding: 24px; text-align: center;">
            <h1 style="color: #ffffff; margin: 0; font-size: 22px;">دوا 24 | Dawa24</h1>
          </td>
        </tr>
        <tr>
          <td style="padding: 32px 24px; color: #1e293b; line-height: 1.6;">
            <div style="background: #fef2f2; border-right: 4px solid #ef4444; padding: 12px 16px; border-radius: 6px; margin-bottom: 20px;">
              <strong style="color: #991b1b; font-size: 15px;">🔒 تنبيه أمني بشأن حسابك</strong>
            </div>
            <p style="font-size: 15px; margin: 0 0 16px;">مرحباً،</p>
            <p style="font-size: 14px; margin: 0 0 16px;">
              نود إحاطتك بأنه تم تحديث وتغيير كلمة المرور الخاصة بحسابك في منصة دوا 24 بنجاح في:
              <strong style="font-family: monospace;">%s</strong>
            </p>
            <p style="font-size: 14px; margin: 0 0 20px; color: #475569;">
              إذا قمت أنت بهذا الإجراء، يمكنك تجاهل هذه الرسالة بأمان.
            </p>
            <div style="background: #f8fafc; border: 1px solid #e2e8f0; padding: 16px; border-radius: 8px; font-size: 13px; color: #64748b;">
              <strong>هل لم تكن أنت؟</strong><br>
              إذا لم تكن أنت من قام بهذا التعديل، يرجى إعادة تعيين كلمة المرور فوراً عبر المنصة أو التواصل مع فريق الدعم الفني:
              <a href="mailto:support@dawa24.com" style="color: #2563eb;">support@dawa24.com</a>
            </div>
          </td>
        </tr>
        <tr>
          <td style="background: #f8fafc; padding: 16px 24px; text-align: center; font-size: 12px; color: #94a3b8; border-top: 1px solid #e2e8f0;">
            منظومة دوا 24 الرقمية للتوريدات الدوائية &copy; 2026
          </td>
        </tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`, nowStr)

	h.safeGo("send-password-change-alert", func() {
		_ = h.Mailer().Send(context.Background(), mailer.Message{
			To:      recipientEmail,
			Subject: subject,
			Text:    text,
			HTML:    html,
		})
	})
}
