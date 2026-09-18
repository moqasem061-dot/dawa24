package ui

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/mailer"
	"github.com/muhiya/dawa24-store/internal/platform/telegramgateway"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *UIHandler) registerPreApprovalSettingsRoutes(r chi.Router) {
	r.Get("/settings", h.SettingsIndex)
	r.Get("/settings/profile", redirectToSettingsTab("profile"))
	r.Post("/settings/profile", h.SettingsProfileSubmit)
	r.Post("/settings/password", h.SettingsPasswordSubmit)
	r.Post("/settings/password/send-otp", h.SettingsPasswordSendOTP)
	r.Get("/settings/addresses", redirectToSettingsTab("profile"))
	r.Get("/settings/security", redirectToSettingsTab("security"))
	r.Get("/settings/preferences", redirectToSettingsTab("preferences"))
	r.Post("/settings/addresses", h.SettingsAddressSubmit)
	r.Post("/settings/addresses/{id}/delete", h.SettingsAddressDeleteSubmit)
	r.Post("/settings/security/revoke", h.SettingsSessionRevokeSubmit)
	r.Post("/settings/sessions/revoke", h.SettingsSessionRevokeSubmit)
	r.Post("/settings/delete-request", h.SettingsDeleteRequestSubmit)
	r.Post("/settings/delete-request/cancel", h.SettingsAccountDeletionCancelSubmit)
	r.Post("/settings/organization/delete-request/cancel", h.SettingsOrgDeletionCancelSubmit)
	r.Post("/settings/preferences", h.SettingsPreferencesSubmit)

	// Profile verification endpoints
	r.Post("/settings/profile/send-email-otp", h.SettingsProfileSendEmailOTP)
	r.Post("/settings/profile/verify-email-otp", h.SettingsProfileVerifyEmailOTP)
	r.Post("/settings/profile/send-phone-otp", h.SettingsProfileSendPhoneOTP)
	r.Post("/settings/profile/verify-phone-otp", h.SettingsProfileVerifyPhoneOTP)

	// Telegram & WhatsApp linking
	r.Get("/settings/telegram", h.SettingsTelegramCard)
	r.Post("/settings/telegram/link", h.SettingsTelegramLinkSubmit)
	r.Post("/settings/telegram/confirm", h.SettingsTelegramConfirmSubmit)
	r.Post("/settings/telegram/unlink", h.SettingsTelegramUnlinkSubmit)
	r.Post("/settings/telegram/notify", h.SettingsTelegramNotifySubmit)
	r.Get("/settings/whatsapp", h.SettingsWhatsAppCard)
	r.Post("/settings/whatsapp/link", h.SettingsWhatsAppLinkSubmit)
	r.Post("/settings/whatsapp/confirm", h.SettingsWhatsAppConfirmSubmit)
	r.Post("/settings/whatsapp/unlink", h.SettingsWhatsAppUnlinkSubmit)
	r.Post("/settings/whatsapp/notify", h.SettingsWhatsAppNotifySubmit)
}

// SettingsProfileSendEmailOTP sends an OTP to verify the logged-in user's email.
func (h *UIHandler) SettingsProfileSendEmailOTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, sendEmailOTPResp{OK: false, Error: "unauthorized"})
		return
	}

	rawEmail := strings.TrimSpace(r.PostFormValue("email"))
	if rawEmail == "" && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var req sendEmailOTPReq
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			rawEmail = strings.TrimSpace(req.Email)
		}
	}

	if h.idSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, sendEmailOTPResp{OK: false, Error: "service_unavailable", Message: i18n.T(lang, "common.service_unavailable")})
		return
	}

	me, err := h.idSvc.GetMe(ctx, actor.UserID, nil)
	if err != nil || me == nil || me.User == nil {
		writeJSON(w, http.StatusBadRequest, sendEmailOTPResp{OK: false, Error: "user_not_found", Message: i18n.T(lang, "common.service_unavailable")})
		return
	}

	if rawEmail == "" {
		rawEmail = me.User.Email
	}

	cleanEmail := identity.NormalizeEmail(rawEmail)
	if cleanEmail == "" || !strings.Contains(cleanEmail, "@") || !strings.Contains(cleanEmail, ".") {
		writeJSON(w, http.StatusBadRequest, sendEmailOTPResp{OK: false, Error: "invalid_email", Message: i18n.T(lang, "auth.email.invalid_email")})
		return
	}

	if !strings.EqualFold(cleanEmail, me.User.Email) {
		sysCtx := database.AsSystem(ctx)
		existingUser, err := h.idSvc.GetUserByEmail(sysCtx, cleanEmail)
		if err == nil && existingUser != nil && existingUser.ID != actor.UserID {
			writeJSON(w, http.StatusConflict, sendEmailOTPResp{OK: false, Error: "already_registered", Message: i18n.T(lang, "auth.email.already_registered")})
			return
		}
	}

	code, cooldown, err := h.EmailOTP().GenerateOTP(cleanEmail, "profile_verification", 15*time.Minute)
	if err != nil {
		if errors.Is(err, mailer.ErrOTPInCooldown) {
			writeJSON(w, http.StatusTooManyRequests, sendEmailOTPResp{OK: false, Error: "cooldown", Cooldown: cooldown, Message: i18n.T(lang, "auth.email.cooldown")})
			return
		}
		if errors.Is(err, mailer.ErrOTPHourlyExceeded) {
			writeJSON(w, http.StatusTooManyRequests, sendEmailOTPResp{OK: false, Error: "rate_limited", Message: i18n.T(lang, "auth.email.rate_limit")})
			return
		}
		h.log.ErrorContext(ctx, "failed to generate profile email otp", "email", cleanEmail, "error", err)
		writeJSON(w, http.StatusInternalServerError, sendEmailOTPResp{OK: false, Error: "generation_failed", Message: i18n.T(lang, "auth.email.error_sending")})
		return
	}

	if err := h.Mailer().SendOTP(ctx, cleanEmail, code, "verification", lang); err != nil {
		h.log.WarnContext(ctx, "failed to send profile email otp", "email", cleanEmail, "error", err)
		writeJSON(w, http.StatusBadGateway, sendEmailOTPResp{OK: false, Error: "send_failed", Message: i18n.T(lang, "auth.email.error_sending")})
		return
	}

	resp := sendEmailOTPResp{OK: true, Email: cleanEmail, ExpiresIn: 900, Cooldown: 60, Message: i18n.T(lang, "auth.email.code_sent")}
	if h.Mailer().IsMock() {
		resp.MockCode = code
	}
	writeJSON(w, http.StatusOK, resp)
}

// SettingsProfileVerifyEmailOTP checks the OTP and marks user's email as verified.
func (h *UIHandler) SettingsProfileVerifyEmailOTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, verifyEmailOTPResp{OK: false, Error: "unauthorized"})
		return
	}

	rawEmail := strings.TrimSpace(r.PostFormValue("email"))
	code := strings.TrimSpace(r.PostFormValue("code"))
	if rawEmail == "" && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var req verifyEmailOTPReq
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			rawEmail = strings.TrimSpace(req.Email)
			code = strings.TrimSpace(req.Code)
		}
	}

	if rawEmail == "" && h.idSvc != nil {
		if me, err := h.idSvc.GetMe(ctx, actor.UserID, nil); err == nil && me != nil && me.User != nil {
			rawEmail = me.User.Email
		}
	}

	cleanEmail := identity.NormalizeEmail(rawEmail)
	if cleanEmail == "" {
		writeJSON(w, http.StatusBadRequest, verifyEmailOTPResp{OK: false, Error: "invalid_email", Message: i18n.T(lang, "auth.email.invalid_email")})
		return
	}
	if code == "" {
		writeJSON(w, http.StatusBadRequest, verifyEmailOTPResp{OK: false, Error: "missing_code", Message: i18n.T(lang, "auth.email.invalid_code")})
		return
	}

	valid, err := h.EmailOTP().VerifyOTP(cleanEmail, "profile_verification", code)
	if !valid || err != nil {
		errKey, errCode := "auth.email.invalid_code", "invalid_code"
		status := http.StatusBadRequest
		if errors.Is(err, mailer.ErrOTPMaxAttempts) {
			status, errKey, errCode = http.StatusTooManyRequests, "auth.email.max_attempts", "max_attempts_exceeded"
		} else if errors.Is(err, mailer.ErrOTPExpired) {
			errKey, errCode = "auth.email.expired_code", "expired_code"
		}
		writeJSON(w, status, verifyEmailOTPResp{OK: false, Error: errCode, Message: i18n.Translate(lang, errKey)})
		return
	}

	if h.idSvc != nil {
		if err := h.idSvc.VerifyUserEmail(ctx, actor.UserID, cleanEmail); err != nil {
			h.log.ErrorContext(ctx, "failed to update user email verification", "user_id", actor.UserID, "error", err)
			writeJSON(w, http.StatusInternalServerError, verifyEmailOTPResp{OK: false, Error: "update_failed", Message: i18n.T(lang, "common.service_unavailable")})
			return
		}
	}

	if h.resolver != nil {
		h.resolver.Invalidate(actor.UserID, actor.OrganizationID)
	}

	writeJSON(w, http.StatusOK, verifyEmailOTPResp{OK: true, Verified: true, Email: cleanEmail, Message: i18n.T(lang, "auth.email.verified")})
}

// SettingsProfileSendPhoneOTP sends an OTP via Telegram to verify the user's phone.
func (h *UIHandler) SettingsProfileSendPhoneOTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, sendOTPResp{OK: false, Error: "unauthorized"})
		return
	}

	rawPhone := strings.TrimSpace(r.PostFormValue("phone"))
	if rawPhone == "" && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var req sendOTPReq
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			rawPhone = strings.TrimSpace(req.Phone)
		}
	}

	if rawPhone == "" && h.idSvc != nil {
		if me, err := h.idSvc.GetMe(ctx, actor.UserID, nil); err == nil && me != nil && me.User != nil {
			rawPhone = me.User.Phone
		}
	}

	normPhone, err := telegramgateway.NormalizePhone(rawPhone)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, sendOTPResp{OK: false, Error: "invalid_phone", Message: i18n.T(lang, "auth.telegram.invalid_phone")})
		return
	}

	limiter := h.telegramLimiter()
	if inCooldown, remaining := limiter.CheckSendCooldown(normPhone); inCooldown {
		writeJSON(w, http.StatusTooManyRequests, sendOTPResp{OK: false, Error: "cooldown", Cooldown: remaining, Message: i18n.T(lang, "auth.telegram.cooldown")})
		return
	}

	if limiter.CheckHourlySendLimit(normPhone, 5) {
		writeJSON(w, http.StatusTooManyRequests, sendOTPResp{OK: false, Error: "rate_limited", Message: i18n.T(lang, "auth.telegram.rate_limit")})
		return
	}

	result, err := h.telegramGateway().SendVerification(ctx, normPhone)
	if err != nil {
		h.log.WarnContext(ctx, "send telegram otp failed for profile", "phone", normPhone, "error", err)
		writeJSON(w, http.StatusBadGateway, sendOTPResp{OK: false, Error: "send_failed", Message: i18n.T(lang, "auth.telegram.error_sending")})
		return
	}

	limiter.RecordSend(normPhone, 60*time.Second)
	writeJSON(w, http.StatusOK, sendOTPResp{
		OK:        true,
		RequestID: result.RequestID,
		Phone:     result.PhoneNumber,
		ExpiresIn: result.ExpiresIn,
		Cooldown:  60,
		MockCode:  result.MockCode,
		Message:   i18n.T(lang, "auth.telegram.code_sent"),
	})
}

// SettingsProfileVerifyPhoneOTP validates the Telegram OTP and marks phone as verified.
func (h *UIHandler) SettingsProfileVerifyPhoneOTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, verifyOTPResp{OK: false, Error: "unauthorized"})
		return
	}

	rawPhone := strings.TrimSpace(r.PostFormValue("phone"))
	requestID := strings.TrimSpace(r.PostFormValue("request_id"))
	code := strings.TrimSpace(r.PostFormValue("code"))
	if rawPhone == "" && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var req verifyOTPReq
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			rawPhone = strings.TrimSpace(req.Phone)
			requestID = strings.TrimSpace(req.RequestID)
			code = strings.TrimSpace(req.Code)
		}
	}

	if rawPhone == "" && h.idSvc != nil {
		if me, err := h.idSvc.GetMe(ctx, actor.UserID, nil); err == nil && me != nil && me.User != nil {
			rawPhone = me.User.Phone
		}
	}

	normPhone, err := telegramgateway.NormalizePhone(rawPhone)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, verifyOTPResp{OK: false, Error: "invalid_phone", Message: i18n.T(lang, "auth.telegram.invalid_phone")})
		return
	}

	if requestID == "" || code == "" {
		writeJSON(w, http.StatusBadRequest, verifyOTPResp{OK: false, Error: "missing_parameters", Message: i18n.T(lang, "auth.telegram.invalid_code")})
		return
	}

	limiter := h.telegramLimiter()
	if !limiter.CheckAndRecordVerifyAttempt(requestID, 5) {
		writeJSON(w, http.StatusTooManyRequests, verifyOTPResp{OK: false, Error: "max_attempts_exceeded", Message: i18n.T(lang, "auth.telegram.max_attempts")})
		return
	}

	result, err := h.telegramGateway().CheckVerification(ctx, requestID, code)
	if err != nil {
		h.log.WarnContext(ctx, "telegram gateway check failed for profile", "phone", normPhone, "request_id", requestID, "error", err)
		writeJSON(w, http.StatusBadGateway, verifyOTPResp{OK: false, Error: "verify_failed", Message: i18n.T(lang, "auth.telegram.error_verifying")})
		return
	}

	if !result.Valid {
		writeJSON(w, http.StatusBadRequest, verifyOTPResp{OK: false, Error: "invalid_code", Message: i18n.T(lang, "auth.telegram.invalid_code")})
		return
	}

	limiter.ClearRequest(requestID)

	if h.idSvc != nil {
		if err := h.idSvc.VerifyUserPhone(ctx, actor.UserID, normPhone); err != nil {
			h.log.ErrorContext(ctx, "failed to update user phone verification", "user_id", actor.UserID, "error", err)
			writeJSON(w, http.StatusInternalServerError, verifyOTPResp{OK: false, Error: "update_failed", Message: i18n.T(lang, "common.service_unavailable")})
			return
		}
	}

	if h.resolver != nil {
		h.resolver.Invalidate(actor.UserID, actor.OrganizationID)
	}

	writeJSON(w, http.StatusOK, verifyOTPResp{OK: true, Verified: true, Phone: normPhone, Message: i18n.T(lang, "auth.telegram.verified")})
}
