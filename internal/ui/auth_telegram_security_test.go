package ui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/telegramgateway"
	"github.com/moqasem061-dot/dawa24/internal/ui"
)

type verifiedPhoneGateway struct{ phone string }

func (verifiedPhoneGateway) IsMock() bool { return false }
func (g verifiedPhoneGateway) SendVerification(context.Context, string) (*telegramgateway.SendResult, error) {
	return &telegramgateway.SendResult{RequestID: "fixture-request", PhoneNumber: g.phone, ExpiresIn: 300}, nil
}
func (g verifiedPhoneGateway) CheckVerification(context.Context, string, string) (*telegramgateway.CheckResult, error) {
	return &telegramgateway.CheckResult{Valid: true, Status: "code_valid", PhoneNumber: g.phone}, nil
}

func telegramSecurityHandler() *ui.UIHandler {
	h := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetSessionSecret("fixture-session-secret-at-least-32-bytes")
	return h
}

func verificationRequest(phone string) *http.Request {
	form := url.Values{"phone": {phone}, "request_id": {"fixture-request"}, "code": {"123456"}}
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/telegram/verify-otp", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

func TestTelegramVerificationRejectsDifferentPhoneAndReplay(t *testing.T) {
	h := telegramSecurityHandler()
	h.SetTelegramGatewayClient(verifiedPhoneGateway{phone: "+201012345678"})
	for _, scenario := range []struct {
		phone  string
		status int
	}{
		{"01123456789", http.StatusBadRequest},
		{"01012345678", http.StatusOK},
		{"01012345678", http.StatusTooManyRequests},
	} {
		rr := httptest.NewRecorder()
		h.VerifyTelegramOTP(rr, verificationRequest(scenario.phone))
		if rr.Code != scenario.status {
			t.Fatalf("phone=%s status=%d want=%d body=%s", scenario.phone, rr.Code, scenario.status, rr.Body)
		}
	}
}

func TestProfileTelegramVerificationRejectsDifferentPhone(t *testing.T) {
	h := telegramSecurityHandler()
	h.SetTelegramGatewayClient(verifiedPhoneGateway{phone: "+201012345678"})
	r := verificationRequest("01123456789")
	r = r.WithContext(authctx.WithActor(r.Context(), authctx.Actor{UserID: 42, OrganizationID: 5}))
	rr := httptest.NewRecorder()
	h.SettingsProfileVerifyPhoneOTP(rr, r)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unowned phone accepted=%d %s", rr.Code, rr.Body)
	}
}

func TestProductionTelegramVerificationRefusesMock(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	h := telegramSecurityHandler()
	for _, endpoint := range []http.HandlerFunc{h.SendTelegramOTP, h.VerifyTelegramOTP} {
		rr := httptest.NewRecorder()
		endpoint(rr, verificationRequest("01012345678"))
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("mock available=%d %s", rr.Code, rr.Body)
		}
		if rr.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("verification response cacheable")
		}
	}
}
