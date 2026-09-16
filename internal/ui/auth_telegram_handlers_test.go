package ui_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/telegramgateway"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestSendTelegramOTP(t *testing.T) {
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetSessionSecret("test_session_secret_with_32_bytes_len!")

	// 1. Invalid phone number
	req := httptest.NewRequest(http.MethodPost, "/auth/telegram/send-otp", strings.NewReader("phone=invalid"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.SendTelegramOTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 for invalid phone, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}
	if resp["ok"] == true {
		t.Fatal("Expected ok to be false")
	}

	// 2. Valid Egyptian phone number
	req = httptest.NewRequest(http.MethodPost, "/auth/telegram/send-otp", strings.NewReader("phone=01012345678"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	h.SendTelegramOTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 for valid phone, got %d: %s", rr.Code, rr.Body.String())
	}

	resp = nil
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}
	if resp["ok"] != true {
		t.Fatalf("Expected ok to be true, got %v", resp)
	}
	if resp["phone"] != "+201012345678" {
		t.Fatalf("Expected normalized phone +201012345678, got %v", resp["phone"])
	}
	reqID, _ := resp["request_id"].(string)
	if reqID == "" {
		t.Fatal("Expected non-empty request_id")
	}

	// 3. Immediate re-send (should trigger cooldown 429)
	req = httptest.NewRequest(http.MethodPost, "/auth/telegram/send-otp", strings.NewReader("phone=01012345678"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	h.SendTelegramOTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("Expected 429 Cooldown for immediate resend, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestVerifyTelegramOTP(t *testing.T) {
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	secret := "test_session_secret_with_32_bytes_len!"
	h.SetSessionSecret(secret)

	// Step 1: Send OTP
	sendReq := httptest.NewRequest(http.MethodPost, "/auth/telegram/send-otp", strings.NewReader("phone=01198765432"))
	sendReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	sendRR := httptest.NewRecorder()
	h.SendTelegramOTP(sendRR, sendReq)

	var sendResp map[string]any
	_ = json.Unmarshal(sendRR.Body.Bytes(), &sendResp)
	reqID := sendResp["request_id"].(string)

	// Step 2: Verify with wrong code
	wrongData := url.Values{
		"phone":      {"01198765432"},
		"request_id": {reqID},
		"code":       {"000000"},
	}
	verifyReq := httptest.NewRequest(http.MethodPost, "/auth/telegram/verify-otp", strings.NewReader(wrongData.Encode()))
	verifyReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	verifyRR := httptest.NewRecorder()
	h.VerifyTelegramOTP(verifyRR, verifyReq)

	if verifyRR.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 for wrong code, got %d: %s", verifyRR.Code, verifyRR.Body.String())
	}

	// Step 3: Verify with correct mock code (123456)
	correctData := url.Values{
		"phone":      {"01198765432"},
		"request_id": {reqID},
		"code":       {"123456"},
	}
	verifyReq = httptest.NewRequest(http.MethodPost, "/auth/telegram/verify-otp", strings.NewReader(correctData.Encode()))
	verifyReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	verifyRR = httptest.NewRecorder()
	h.VerifyTelegramOTP(verifyRR, verifyReq)

	if verifyRR.Code != http.StatusOK {
		t.Fatalf("Expected 200 for correct code, got %d: %s", verifyRR.Code, verifyRR.Body.String())
	}

	var verifyResp map[string]any
	if err := json.Unmarshal(verifyRR.Body.Bytes(), &verifyResp); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}
	if verifyResp["verified"] != true {
		t.Fatalf("Expected verified=true, got %v", verifyResp)
	}
	token, _ := verifyResp["token"].(string)
	if token == "" {
		t.Fatal("Expected signed verification token in response")
	}

	// Step 4: Verify the minted token cryptographically
	valid, err := telegramgateway.VerifyPhoneToken(token, "+201198765432", secret)
	if err != nil || !valid {
		t.Fatalf("Minted token failed cryptographic verification: valid=%v, err=%v", valid, err)
	}

	// Step 5: Verify anti-tamper check (token for phone A used with phone B)
	valid, err = telegramgateway.VerifyPhoneToken(token, "+201011112222", secret)
	if valid || err != telegramgateway.ErrTokenPhoneMismatch {
		t.Fatalf("Anti-tamper check failed: valid=%v, err=%v", valid, err)
	}
}

func TestRegisterSubmit_PhoneVerificationRequired(t *testing.T) {
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	secret := "test_session_secret_with_32_bytes_len!"
	h.SetSessionSecret(secret)

	// 1. Submit registration WITHOUT verified_phone_token
	formData := url.Values{
		"name":         {"Dr. Test User"},
		"email":        {"test@example.com"},
		"phone":        {"01012345678"},
		"password":     {"Str0ng!Passw0rd"},
		"account_type": {"customer"},
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.RegisterSubmit(rr, req)

	// Should re-render register page with validation error, NOT redirect to session/next
	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 (re-render form with error), got status %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "auth.telegram.phone_required") && !strings.Contains(body, "يرجى تأكيد رقم الهاتف") {
		t.Fatalf("Expected phone verification required error in response, got: %s", body)
	}

	// 2. Submit registration with token verified for a DIFFERENT phone number (tampering attempt)
	diffToken, err := telegramgateway.SignPhoneToken("01099999999", secret, 10*time.Minute)
	if err != nil {
		t.Fatalf("Failed to sign token: %v", err)
	}

	formData.Set("verified_phone_token", diffToken)
	req = httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	h.RegisterSubmit(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200 (re-render form with error) on tampered phone, got status %d", rr.Code)
	}
	body = rr.Body.String()
	if !strings.Contains(body, "auth.telegram.phone_required") && !strings.Contains(body, "يرجى تأكيد رقم الهاتف") {
		t.Fatalf("Expected phone verification error on tampered phone, got: %s", body)
	}
}
