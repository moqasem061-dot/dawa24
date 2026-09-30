package ui

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/platform/telegramgateway"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

func (h *UIHandler) telegramGateway() telegramgateway.Client {
	return h.tgGatewayClient
}

func (h *UIHandler) telegramLimiter() *telegramgateway.OTPRateLimiter {
	return h.tgLimiter
}

func (h *UIHandler) telegramVerificationAvailable() bool {
	return h.tgGatewayClient != nil && (!isProductionEnv() || !h.tgGatewayClient.IsMock())
}

func (h *UIHandler) phoneSecret() string {
	if h.sessionSecret != "" {
		return h.sessionSecret
	}
	// No configured secret: a per-process random one. It was a constant in the
	// source, which anyone could read and use to sign a password-reset token.
	return string(h.mfaPendingKey())
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
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil { // nolint:errswallow: a body that is not JSON falls through to the validation below
			rawPhone = strings.TrimSpace(req.Phone)
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if !h.telegramVerificationAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, sendOTPResp{Error: "service_unavailable", Message: i18n.T(lang, "common.service_unavailable")})
		return
	}

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
	if remaining, err := limiter.ReserveSend(normPhone); err != nil {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(sendOTPResp{
			OK:       false,
			Error:    "cooldown",
			Cooldown: remaining,
			Message:  i18n.T(lang, "auth.telegram.cooldown"),
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

	mockCode := ""
	if !isProductionEnv() && devShowMockOTP() {
		mockCode = result.MockCode
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(sendOTPResp{
		OK:        true,
		RequestID: result.RequestID,
		Phone:     result.PhoneNumber,
		ExpiresIn: result.ExpiresIn,
		Cooldown:  60,
		MockCode:  mockCode,
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
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil { // nolint:errswallow: a body that is not JSON falls through to the validation below
			rawPhone = strings.TrimSpace(req.Phone)
			requestID = strings.TrimSpace(req.RequestID)
			code = strings.TrimSpace(req.Code)
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if !h.telegramVerificationAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, verifyOTPResp{Error: "service_unavailable", Message: i18n.T(lang, "common.service_unavailable")})
		return
	}

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

	verifiedPhone, phoneErr := telegramgateway.NormalizePhone(result.PhoneNumber)
	if !result.Valid || phoneErr != nil || verifiedPhone != normPhone || !limiter.ConsumeRequest(requestID) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(verifyOTPResp{
			OK:      false,
			Error:   "invalid_code",
			Message: i18n.T(lang, "auth.telegram.invalid_code"),
		})
		return
	}

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
