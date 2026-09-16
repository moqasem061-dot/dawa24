package ui

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/mailer"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type sendEmailOTPReq struct {
	Email string `json:"email"`
}

type sendEmailOTPResp struct {
	OK        bool   `json:"ok"`
	Email     string `json:"email,omitempty"`
	ExpiresIn int    `json:"expires_in,omitempty"`
	Cooldown  int    `json:"cooldown,omitempty"`
	MockCode  string `json:"mock_code,omitempty"`
	Error     string `json:"error,omitempty"`
	Message   string `json:"message,omitempty"`
}

// SendEmailOTP handles POST /auth/email/send-otp for registration verification.
func (h *UIHandler) SendEmailOTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	rawEmail := strings.TrimSpace(r.PostFormValue("email"))
	if rawEmail == "" && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var req sendEmailOTPReq
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			rawEmail = strings.TrimSpace(req.Email)
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	cleanEmail := identity.NormalizeEmail(rawEmail)
	if cleanEmail == "" || !strings.Contains(cleanEmail, "@") || !strings.Contains(cleanEmail, ".") {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(sendEmailOTPResp{
			OK:      false,
			Error:   "invalid_email",
			Message: i18n.T(lang, "auth.email.invalid_email"),
		})
		return
	}

	// Check if this email is already registered in the system
	if h.idSvc != nil {
		sysCtx := database.AsSystem(ctx)
		existingUser, err := h.idSvc.GetUserByEmail(sysCtx, cleanEmail)
		if err == nil && existingUser != nil {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(sendEmailOTPResp{
				OK:      false,
				Error:   "already_registered",
				Message: i18n.T(lang, "auth.email.already_registered"),
			})
			return
		}
	}

	engine := h.EmailOTP()
	code, cooldown, err := engine.GenerateOTP(cleanEmail, "registration", 15*time.Minute)
	if err != nil {
		if errors.Is(err, mailer.ErrOTPInCooldown) {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(sendEmailOTPResp{
				OK:       false,
				Error:    "cooldown",
				Cooldown: cooldown,
				Message:  i18n.T(lang, "auth.email.cooldown"),
			})
			return
		}
		if errors.Is(err, mailer.ErrOTPHourlyExceeded) {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(sendEmailOTPResp{
				OK:      false,
				Error:   "rate_limited",
				Message: i18n.T(lang, "auth.email.rate_limit"),
			})
			return
		}
		h.log.ErrorContext(ctx, "failed to generate email otp", "email", cleanEmail, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(sendEmailOTPResp{
			OK:      false,
			Error:   "generation_failed",
			Message: i18n.T(lang, "auth.email.error_sending"),
		})
		return
	}

	// Deliver 6-digit OTP via mailer
	err = h.Mailer().SendOTP(ctx, cleanEmail, code, "registration", lang)
	if err != nil {
		h.log.WarnContext(ctx, "failed to send registration email otp", "email", cleanEmail, "error", err)
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(sendEmailOTPResp{
			OK:      false,
			Error:   "send_failed",
			Message: i18n.T(lang, "auth.email.error_sending"),
		})
		return
	}

	resp := sendEmailOTPResp{
		OK:        true,
		Email:     cleanEmail,
		ExpiresIn: 900,
		Cooldown:  60,
		Message:   i18n.T(lang, "auth.email.code_sent"),
	}

	// In test/mock mode without configured SMTP, surface the code for local dev
	if h.Mailer().IsMock() {
		resp.MockCode = code
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

type verifyEmailOTPReq struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type verifyEmailOTPResp struct {
	OK       bool   `json:"ok"`
	Verified bool   `json:"verified"`
	Email    string `json:"email,omitempty"`
	Token    string `json:"token,omitempty"`
	Error    string `json:"error,omitempty"`
	Message  string `json:"message,omitempty"`
}

// VerifyEmailOTP handles POST /auth/email/verify-otp for registration verification.
func (h *UIHandler) VerifyEmailOTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	rawEmail := strings.TrimSpace(r.PostFormValue("email"))
	code := strings.TrimSpace(r.PostFormValue("code"))

	if rawEmail == "" && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var req verifyEmailOTPReq
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			rawEmail = strings.TrimSpace(req.Email)
			code = strings.TrimSpace(req.Code)
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	cleanEmail := identity.NormalizeEmail(rawEmail)
	if cleanEmail == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(verifyEmailOTPResp{
			OK:      false,
			Error:   "invalid_email",
			Message: i18n.T(lang, "auth.email.invalid_email"),
		})
		return
	}

	if code == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(verifyEmailOTPResp{
			OK:      false,
			Error:   "missing_code",
			Message: i18n.T(lang, "auth.email.invalid_code"),
		})
		return
	}

	engine := h.EmailOTP()
	valid, err := engine.VerifyOTP(cleanEmail, "registration", code)
	if !valid || err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errKey := "auth.email.invalid_code"
		errCode := "invalid_code"
		if errors.Is(err, mailer.ErrOTPMaxAttempts) {
			w.WriteHeader(http.StatusTooManyRequests)
			errKey = "auth.email.max_attempts"
			errCode = "max_attempts_exceeded"
		} else if errors.Is(err, mailer.ErrOTPExpired) {
			errKey = "auth.email.expired_code"
			errCode = "expired_code"
		}
		_ = json.NewEncoder(w).Encode(verifyEmailOTPResp{
			OK:      false,
			Error:   errCode,
			Message: i18n.Translate(lang, errKey),
		})
		return
	}

	token, err := mailer.SignEmailToken(cleanEmail, h.phoneSecret(), 30*time.Minute)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to sign verified email token", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(verifyEmailOTPResp{
			OK:      false,
			Error:   "token_sign_failed",
			Message: i18n.T(lang, "auth.email.invalid_code"),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(verifyEmailOTPResp{
		OK:       true,
		Verified: true,
		Email:    cleanEmail,
		Token:    token,
		Message:  i18n.T(lang, "auth.email.verified"),
	})
}
