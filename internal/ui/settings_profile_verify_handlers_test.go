package ui_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui"
)

func TestSettingsProfileVerificationHandlers_Unauthorized(t *testing.T) {
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	// Send Email OTP without authctx
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/settings/profile/send-email-otp", strings.NewReader("email=test@dawa24.eg"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.SettingsProfileSendEmailOTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", rr.Code)
	}

	// Verify Email OTP without authctx
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/settings/profile/verify-email-otp", strings.NewReader("email=test@dawa24.eg&code=123456"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	h.SettingsProfileVerifyEmailOTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", rr.Code)
	}

	// Send Phone OTP without authctx
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/settings/profile/send-phone-otp", strings.NewReader("phone=01012345678"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	h.SettingsProfileSendPhoneOTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", rr.Code)
	}

	// Verify Phone OTP without authctx
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/settings/profile/verify-phone-otp", strings.NewReader("phone=01012345678&request_id=xyz&code=123456"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	h.SettingsProfileVerifyPhoneOTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", rr.Code)
	}
}

func TestSettingsProfilePhoneVerification_Flow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := &mockIdentityRepoProfile{
		user: &identity.User{
			ID:     1,
			Phone:  "01012345678",
			Name:   i18n.New("د. أحمد", "Dr. Ahmed"),
			Role:   "customer",
			Status: identity.StatusActive,
		},
	}
	idSvc := identity.NewService(repo, nil, logger)

	h := ui.NewUIHandler(nil, nil, nil, nil, nil, idSvc, nil, nil, nil, nil, nil, nil, nil, nil, logger)
	h.SetSessionSecret("test_session_secret_with_32_bytes_len!")

	actor := authctx.Actor{
		UserID:         1,
		OrganizationID: 1,
		Role:           "customer",
	}

	// 1. Send OTP for phone
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/settings/profile/send-phone-otp", strings.NewReader("phone=01012345678"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(context.Background(), actor))
	rr := httptest.NewRecorder()
	h.SettingsProfileSendPhoneOTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var sendResp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &sendResp); err != nil {
		t.Fatalf("failed to decode send response: %v", err)
	}
	if sendResp["ok"] != true {
		t.Fatalf("expected ok true, got: %v", sendResp)
	}
	reqID, _ := sendResp["request_id"].(string)
	if reqID == "" {
		t.Fatalf("expected non-empty request_id")
	}

	// 2. Verify with wrong code
	verifyBody := "phone=01012345678&request_id=" + reqID + "&code=000000"
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/settings/profile/verify-phone-otp", strings.NewReader(verifyBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(context.Background(), actor))
	rr = httptest.NewRecorder()
	h.SettingsProfileVerifyPhoneOTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for wrong code, got %d: %s", rr.Code, rr.Body.String())
	}

	// 3. Verify with valid mock code (123456)
	verifyValidBody := "phone=01012345678&request_id=" + reqID + "&code=123456"
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/settings/profile/verify-phone-otp", strings.NewReader(verifyValidBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(context.Background(), actor))
	rr = httptest.NewRecorder()
	h.SettingsProfileVerifyPhoneOTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid code, got %d: %s", rr.Code, rr.Body.String())
	}
	if repo.user.PhoneVerifiedAt == nil {
		t.Fatalf("expected user.PhoneVerifiedAt to be updated")
	}
}

func TestSettingsProfileEmailVerification_Flow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := &mockIdentityRepoProfile{
		user: &identity.User{
			ID:     1,
			Email:  "test@dawa24.eg",
			Name:   i18n.New("د. أحمد", "Dr. Ahmed"),
			Role:   "customer",
			Status: identity.StatusActive,
		},
	}
	idSvc := identity.NewService(repo, nil, logger)

	h := ui.NewUIHandler(nil, nil, nil, nil, nil, idSvc, nil, nil, nil, nil, nil, nil, nil, nil, logger)
	h.SetSessionSecret("test_session_secret_with_32_bytes_len!")

	actor := authctx.Actor{
		UserID:         1,
		OrganizationID: 1,
		Role:           "customer",
	}

	// 1. Send OTP for email
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/settings/profile/send-email-otp", strings.NewReader("email=test@dawa24.eg"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(context.Background(), actor))
	rr := httptest.NewRecorder()
	h.SettingsProfileSendEmailOTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var sendResp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &sendResp); err != nil {
		t.Fatalf("failed to decode send response: %v", err)
	}
	if sendResp["ok"] != true {
		t.Fatalf("expected ok true, got: %v", sendResp)
	}
	mockCode, _ := sendResp["mock_code"].(string)
	if mockCode == "" {
		t.Fatalf("expected mock_code in mock mailer mode")
	}

	// 2. Verify with wrong code
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/settings/profile/verify-email-otp", strings.NewReader("email=test@dawa24.eg&code=000000"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(context.Background(), actor))
	rr = httptest.NewRecorder()
	h.SettingsProfileVerifyEmailOTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for wrong code, got %d: %s", rr.Code, rr.Body.String())
	}

	// 3. Verify with valid mockCode
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/settings/profile/verify-email-otp", strings.NewReader("email=test@dawa24.eg&code="+mockCode))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(context.Background(), actor))
	rr = httptest.NewRecorder()
	h.SettingsProfileVerifyEmailOTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid code, got %d: %s", rr.Code, rr.Body.String())
	}

	var verifyResp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &verifyResp); err != nil {
		t.Fatalf("failed to decode verify response: %v", err)
	}
	if verifyResp["ok"] != true || verifyResp["verified"] != true {
		t.Fatalf("expected ok and verified true, got: %v", verifyResp)
	}
	if repo.user.EmailVerifiedAt == nil {
		t.Fatalf("expected user.EmailVerifiedAt to be updated")
	}
}
