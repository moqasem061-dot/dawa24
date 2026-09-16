package ui

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/telegramgateway"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func (h *UIHandler) telegramGateway() telegramgateway.Client {
	if h.tgGatewayClient != nil {
		return h.tgGatewayClient
	}
	h.tgGatewayClient = telegramgateway.New(telegramgateway.Config{
		MockMode: true,
		Log:      h.log,
	})
	return h.tgGatewayClient
}

func (h *UIHandler) telegramLimiter() *telegramgateway.OTPRateLimiter {
	if h.tgLimiter == nil {
		h.tgLimiter = telegramgateway.NewLimiter()
	}
	return h.tgLimiter
}

func (h *UIHandler) phoneSecret() string {
	if h.sessionSecret != "" {
		return h.sessionSecret
	}
	return "dawa24-verification-fallback-secret-2024"
}

type sendOTPReq struct {
	Phone string `json:"phone"`
}

type sendOTPResp struct {
	OK        bool   `json:"ok"`
	RequestID string `json:"request_id,omitempty"`
	Phone     string `json:"phone,omitempty"`
	ExpiresIn int    `json:"expires_in,omitempty"`
	Cooldown  int    `json:"cooldown,omitempty"`
	MockCode  string `json:"mock_code,omitempty"`
	Error     string `json:"error,omitempty"`
	Message   string `json:"message,omitempty"`
}

// SendTelegramOTP handles POST /auth/telegram/send-otp.
func (h *UIHandler) SendTelegramOTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	rawPhone := strings.TrimSpace(r.PostFormValue("phone"))
	if rawPhone == "" && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var req sendOTPReq
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			rawPhone = strings.TrimSpace(req.Phone)
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	normPhone, err := telegramgateway.NormalizePhone(rawPhone)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(sendOTPResp{
			OK:      false,
			Error:   "invalid_phone",
			Message: i18n.T(lang, "auth.telegram.invalid_phone"),
		})
		return
	}

	limiter := h.telegramLimiter()
	if inCooldown, remaining := limiter.CheckSendCooldown(normPhone); inCooldown {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(sendOTPResp{
			OK:       false,
			Error:    "cooldown",
			Cooldown: remaining,
			Message:  i18n.T(lang, "auth.telegram.cooldown"),
		})
		return
	}

	if limiter.CheckHourlySendLimit(normPhone, 5) {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(sendOTPResp{
			OK:      false,
			Error:   "rate_limited",
			Message: i18n.T(lang, "auth.telegram.rate_limit"),
		})
		return
	}

	client := h.telegramGateway()
	result, err := client.SendVerification(ctx, normPhone)
	if err != nil {
		h.log.WarnContext(ctx, "send telegram otp failed", "phone", normPhone, "error", err)
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(sendOTPResp{
			OK:      false,
			Error:   "send_failed",
			Message: i18n.T(lang, "auth.telegram.error_sending"),
		})
		return
	}

	limiter.RecordSend(normPhone, 60*time.Second)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(sendOTPResp{
		OK:        true,
		RequestID: result.RequestID,
		Phone:     result.PhoneNumber,
		ExpiresIn: result.ExpiresIn,
		Cooldown:  60,
		MockCode:  result.MockCode,
		Message:   i18n.T(lang, "auth.telegram.code_sent"),
	})
}

type verifyOTPReq struct {
	Phone     string `json:"phone"`
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
}

type verifyOTPResp struct {
	OK       bool   `json:"ok"`
	Verified bool   `json:"verified"`
	Phone    string `json:"phone,omitempty"`
	Token    string `json:"token,omitempty"`
	Error    string `json:"error,omitempty"`
	Message  string `json:"message,omitempty"`
}

// VerifyTelegramOTP handles POST /auth/telegram/verify-otp.
func (h *UIHandler) VerifyTelegramOTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

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

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	normPhone, err := telegramgateway.NormalizePhone(rawPhone)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(verifyOTPResp{
			OK:      false,
			Error:   "invalid_phone",
			Message: i18n.T(lang, "auth.telegram.invalid_phone"),
		})
		return
	}

	if requestID == "" || code == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(verifyOTPResp{
			OK:      false,
			Error:   "missing_parameters",
			Message: i18n.T(lang, "auth.telegram.invalid_code"),
		})
		return
	}

	limiter := h.telegramLimiter()
	if !limiter.CheckAndRecordVerifyAttempt(requestID, 5) {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(verifyOTPResp{
			OK:      false,
			Error:   "max_attempts_exceeded",
			Message: i18n.T(lang, "auth.telegram.max_attempts"),
		})
		return
	}

	client := h.telegramGateway()
	result, err := client.CheckVerification(ctx, requestID, code)
	if err != nil {
		h.log.WarnContext(ctx, "telegram gateway check failed", "phone", normPhone, "request_id", requestID, "error", err)
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(verifyOTPResp{
			OK:      false,
			Error:   "verify_failed",
			Message: i18n.T(lang, "auth.telegram.error_verifying"),
		})
		return
	}

	if !result.Valid {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(verifyOTPResp{
			OK:      false,
			Error:   "invalid_code",
			Message: i18n.T(lang, "auth.telegram.invalid_code"),
		})
		return
	}

	limiter.ClearRequest(requestID)

	token, err := telegramgateway.SignPhoneToken(normPhone, h.phoneSecret(), 30*time.Minute)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to sign verified phone token", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(verifyOTPResp{
			OK:      false,
			Error:   "token_sign_failed",
			Message: i18n.T(lang, "auth.telegram.generic_error"),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(verifyOTPResp{
		OK:       true,
		Verified: true,
		Phone:    normPhone,
		Token:    token,
		Message:  i18n.T(lang, "auth.telegram.verified"),
	})
}
