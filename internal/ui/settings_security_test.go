package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

func TestSettingsPasswordChange_WithEmailOTP(t *testing.T) {
	h, idSvc, _, _ := setupTestMFAHandler(t)
	ctx := context.Background()

	// 1. Register test user
	user, _, err := idSvc.Register(ctx, identity.RegisterInput{
		Email:    "security-test@dawa24.eg",
		Password: "OldPassword123!",
		NameAr:   "مستخدم أمان",
		NameEn:   "Security User",
		Role:     "customer",
		Language: i18n.AR,
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	actor := authctx.Actor{
		UserID:  user.ID,
		Email:   user.Email,
		Role:    "customer",
		OrgType: "customer",
	}
	ctxWithActor := authctx.WithActor(ctx, actor)

	// 2. Request OTP code via POST /settings/password/send-otp
	reqOTP := httptest.NewRequestWithContext(context.Background(), "POST", "/settings/password/send-otp", nil).WithContext(ctxWithActor)
	recOTP := httptest.NewRecorder()
	h.SettingsPasswordSendOTP(recOTP, reqOTP)

	if recOTP.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on send-otp, got %d", recOTP.Result().StatusCode)
	}

	var otpResp sendPasswordChangeOTPResp
	if err := json.NewDecoder(recOTP.Body).Decode(&otpResp); err != nil {
		t.Fatalf("failed to decode send-otp response: %v", err)
	}
	if !otpResp.OK {
		t.Fatalf("expected send-otp ok=true, got error: %s", otpResp.Error)
	}

	engine := h.EmailOTP()

	// 3. Attempt password change with invalid OTP -> should be rejected
	formFail := url.Values{
		"new_password":              {"NewSecurePass123!"},
		"new_password_confirmation": {"NewSecurePass123!"},
		"email_otp_code":            {"000000"},
	}
	reqFail := httptest.NewRequestWithContext(context.Background(), "POST", "/settings/password", strings.NewReader(formFail.Encode())).WithContext(ctxWithActor)
	reqFail.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recFail := httptest.NewRecorder()
	h.SettingsPasswordSubmit(recFail, reqFail)

	locFail := recFail.Result().Header.Get("Location")
	if !strings.Contains(locFail, "notice=error") {
		t.Fatalf("expected notice=error with wrong OTP, got %s", locFail)
	}

	// Let's register another user to have fresh cooldown
	user2, _, err := idSvc.Register(ctx, identity.RegisterInput{
		Email:    "fresh-security@dawa24.eg",
		Password: "OldPass123!456",
		NameAr:   "مستخدم جديد",
		Role:     "customer",
	})
	if err != nil {
		t.Fatalf("Register user2 failed: %v", err)
	}
	actor2 := authctx.Actor{UserID: user2.ID, Email: user2.Email, Role: "customer"}
	ctxWithActor2 := authctx.WithActor(ctx, actor2)

	code2, _, err := engine.GenerateOTP(user2.Email, "password_change", 15*time.Minute)
	if err != nil {
		t.Fatalf("GenerateOTP failed: %v", err)
	}

	// 5. Submit valid password change with valid OTP
	formSuccess := url.Values{
		"new_password":              {"SuperSecret123!@#"},
		"new_password_confirmation": {"SuperSecret123!@#"},
		"email_otp_code":            {code2},
	}
	reqSuccess := httptest.NewRequestWithContext(context.Background(), "POST", "/settings/password", strings.NewReader(formSuccess.Encode())).WithContext(ctxWithActor2)
	reqSuccess.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recSuccess := httptest.NewRecorder()
	h.SettingsPasswordSubmit(recSuccess, reqSuccess)

	locSuccess := recSuccess.Result().Header.Get("Location")
	if !strings.Contains(locSuccess, "notice=success") {
		t.Fatalf("expected notice=success with valid OTP, got %s", locSuccess)
	}

	// 6. Verify login with old password FAILS
	_, err = idSvc.Login(ctx, identity.LoginInput{
		Email:    user2.Email,
		Password: "OldPass123!456",
	})
	if err == nil {
		t.Fatalf("expected login with old password to fail")
	}

	// 7. Verify login with new password SUCCEEDS
	loginRes, err := idSvc.Login(ctx, identity.LoginInput{
		Email:    user2.Email,
		Password: "SuperSecret123!@#",
	})
	if err != nil || loginRes == nil || loginRes.User == nil {
		t.Fatalf("expected login with new password to succeed, got %v", err)
	}
}

func TestForgotSubmit_UserFoundAndNotFound(t *testing.T) {
	h, idSvc, _, _ := setupTestMFAHandler(t)
	ctx := context.Background()

	// 1. Non-existent email -> should render user_not_found error
	formNotFound := url.Values{"email": {"nonexistent@dawa24.eg"}}
	reqNotFound := httptest.NewRequestWithContext(context.Background(), "POST", "/auth/forgot", strings.NewReader(formNotFound.Encode()))
	reqNotFound.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recNotFound := httptest.NewRecorder()
	h.ForgotSubmit(recNotFound, reqNotFound)

	// Should not redirect to verify
	if recNotFound.Result().StatusCode == http.StatusSeeOther {
		loc := recNotFound.Result().Header.Get("Location")
		if strings.Contains(loc, "/auth/forgot/verify") {
			t.Fatalf("non-existent email should not redirect to verify page")
		}
	}

	// 2. Existing user -> should redirect to /auth/forgot/verify?email=...&notice=sent
	existingUser, _, err := idSvc.Register(ctx, identity.RegisterInput{
		Email:    "registered-forgot@dawa24.eg",
		Password: "MyPassword123!",
		NameAr:   "مسجل",
		Role:     "customer",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	formFound := url.Values{"email": {strings.ToUpper(existingUser.Email)}} // test case-insensitive lookup
	reqFound := httptest.NewRequestWithContext(context.Background(), "POST", "/auth/forgot", strings.NewReader(formFound.Encode()))
	reqFound.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recFound := httptest.NewRecorder()
	h.ForgotSubmit(recFound, reqFound)

	if recFound.Result().StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 see other, got %d", recFound.Result().StatusCode)
	}
	loc := recFound.Result().Header.Get("Location")
	if !strings.Contains(loc, "/auth/forgot/verify") || !strings.Contains(loc, "notice=sent") {
		t.Fatalf("expected redirect to /auth/forgot/verify with notice=sent, got %s", loc)
	}
}
