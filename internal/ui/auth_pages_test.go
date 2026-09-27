package ui_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	platformadmin "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

func TestAuthLoginPage_Render(t *testing.T) {
	ctx := context.Background()

	var buf bytes.Buffer
	comp := pages.LoginPage("ar", "rtl", "", "")
	if err := comp.Render(ctx, &buf); err != nil {
		t.Fatalf("LoginPage.Render failed: %v", err)
	}

	html := buf.String()

	expectedSnippets := []string{
		"auth-page-wrapper",
		"auth-card",
		"auth-header",
		"auth-title",
		"منصة دوا 24",
		"تسجيل الدخول إلى حسابك الصيدلي أو التجاري",
		"pwd-input-wrap",
		"pwd-toggle-btn",
		"auth-footer",
		"/auth/register",
		"/auth/forgot",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(html, snippet) {
			t.Errorf("Expected LoginPage HTML to contain %q, but not found", snippet)
		}
	}
}

func TestAuthRegisterPage_Render(t *testing.T) {
	ctx := context.Background()

	form := pages.RegisterFormData{
		LegalName:   "صيدلية المستقبل",
		TradeNameAr: "صيدلية المستقبل",
	}

	// A real governorate and two of its cities, so the chained pickers have
	// something to render and the city list can be checked for its parent tag.
	govID := int64(1)
	governorates := []*platformadmin.Governorate{
		{ID: govID, Name: i18n.Text{i18n.AR: "القاهرة", i18n.EN: "Cairo"}, IsActive: true},
		{ID: 2, Name: i18n.Text{i18n.AR: "الجيزة", i18n.EN: "Giza"}, IsActive: true},
	}
	cities := []*platformadmin.City{
		{ID: 10, GovernorateID: &govID, Name: i18n.Text{i18n.AR: "المعادي", i18n.EN: "Maadi"}, IsActive: true},
		{ID: 11, GovernorateID: &govID, Name: i18n.Text{i18n.AR: "مدينة نصر", i18n.EN: "Nasr City"}, IsActive: true},
	}

	var buf bytes.Buffer
	comp := pages.RegisterPage("ar", "rtl", form, cities, governorates)
	if err := comp.Render(ctx, &buf); err != nil {
		t.Fatalf("RegisterPage.Render failed: %v", err)
	}

	html := buf.String()

	expectedSnippets := []string{
		"auth-page-wrapper",
		"auth-card-wide",
		"auth-stepper",
		"onboard-step",
		"type-card",
		"data-account-type=\"customer\"",
		"data-account-type=\"vendor\"",
		"data-account-type=\"job_seeker\"",
		"pwd-strength-bars",
		"pwd-checklist",
		"auth-footer",
		// The two location pickers are searchable comboboxes over the same
		// rows now. They used to be a <select> of 351 cities for job seekers
		// and, inside the map picker, a hard-coded list of 27 governorate
		// names with no ids, which the map matched against as strings.
		"combobox",
		"role=\"combobox\"",
		"name=\"governorate_id\"",
		"name=\"city_id\"",
		"القاهرة",
		"المعادي",
	}

	// A city carries the governorate it belongs to, and the city picker
	// declares that it follows the governorate picker. Together those are what
	// make the two chain. The payload is JSON inside an HTML attribute, so the
	// quotes arrive escaped.
	if !strings.Contains(html, "&#34;parent&#34;:&#34;1&#34;") {
		t.Error("city options do not carry their governorate, so the pickers cannot chain")
	}
	if !strings.Contains(html, "&#34;dependsOn&#34;:&#34;governorate_id&#34;") {
		t.Error("the city picker does not declare that it follows the governorate picker")
	}
	// And no bare select survives for either.
	if strings.Contains(html, `<select id="reg-city-id"`) {
		t.Error("the old city <select> is still rendered")
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(html, snippet) {
			t.Errorf("Expected RegisterPage HTML to contain %q, but not found", snippet)
		}
	}

	// Test optional verification rendering (default false)
	if !strings.Contains(html, `data-require-email="false"`) {
		t.Errorf("Expected data-require-email=\"false\" when RequireEmailVerification is false")
	}
	// With verification off its UI is not rendered at all.
	for _, id := range []string{`id="email-send-otp-btn"`, `id="email-otp-box"`, `id="verified-email-token"`, "تأكيد البريد"} {
		if strings.Contains(html, id) {
			t.Errorf("email verification UI %q rendered while RequireEmailVerification is false", id)
		}
	}
	if !strings.Contains(html, `data-require-phone="false"`) {
		t.Errorf("Expected data-require-phone=\"false\" when RequirePhoneVerification is false")
	}
	for _, id := range []string{`id="tg-send-otp-btn"`, `id="tg-otp-box"`, `id="verified-phone-token"`, "تأكيد Telegram"} {
		if strings.Contains(html, id) {
			t.Errorf("phone verification UI %q rendered while RequirePhoneVerification is false", id)
		}
	}

	// Test required verification rendering
	formReq := form
	formReq.RequireEmailVerification = true
	formReq.RequirePhoneVerification = true
	var bufReq bytes.Buffer
	compReq := pages.RegisterPage("ar", "rtl", formReq, cities, governorates)
	if err := compReq.Render(ctx, &bufReq); err != nil {
		t.Fatalf("RegisterPage.Render with required verification failed: %v", err)
	}
	htmlReq := bufReq.String()
	if !strings.Contains(htmlReq, `data-require-email="true"`) {
		t.Errorf("Expected data-require-email=\"true\" when RequireEmailVerification is true")
	}
	for _, id := range []string{
		`id="email-otp-feedback"`,
		`id="email-otp-feedback-message"`,
		`id="email-already-registered-login"`,
	} {
		if !strings.Contains(htmlReq, id) {
			t.Errorf("duplicate-email feedback UI %q missing", id)
		}
	}
	if !strings.Contains(htmlReq, "تسجيل الدخول") {
		t.Error("duplicate-email feedback is missing its sign-in action")
	}
	feedbackIndex := strings.Index(htmlReq, `id="email-otp-feedback"`)
	otpPanelIndex := strings.Index(htmlReq, `id="email-otp-box"`)
	if feedbackIndex < 0 || otpPanelIndex < 0 || feedbackIndex > otpPanelIndex {
		t.Error("duplicate-email feedback must render outside the hidden OTP panel")
	}
	if !strings.Contains(htmlReq, "تأكيد البريد مطلوب") {
		t.Errorf("Expected 'تأكيد البريد مطلوب' when RequireEmailVerification is true")
	}
	if !strings.Contains(htmlReq, `data-require-phone="true"`) {
		t.Errorf("Expected data-require-phone=\"true\" when RequirePhoneVerification is true")
	}
	if !strings.Contains(htmlReq, "تأكيد Telegram مطلوب") {
		t.Errorf("Expected 'تأكيد Telegram مطلوب' when RequirePhoneVerification is true")
	}
	for _, id := range []string{`id="email-send-otp-btn"`, `id="email-otp-box"`, `id="tg-send-otp-btn"`, `id="tg-otp-box"`} {
		if !strings.Contains(htmlReq, id) {
			t.Errorf("verification UI %q missing while verification is required", id)
		}
	}
}

func TestMFAVerifyPage_Render(t *testing.T) {
	ctx := context.Background()

	var buf bytes.Buffer
	comp := pages.MFAVerifyPage("ar", "rtl", "pharmacist@dawa24.eg", "", "/dashboard")
	if err := comp.Render(ctx, &buf); err != nil {
		t.Fatalf("MFAVerifyPage.Render failed: %v", err)
	}

	html := buf.String()

	expectedSnippets := []string{
		"auth-page-wrapper",
		"auth-card",
		"auth-totp-input",
		"التحقق بخطوتين (MFA)",
		"pharmacist@dawa24.eg",
		"Google Authenticator",
		"mfa-recovery-code",
		"auth-footer",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(html, snippet) {
			t.Errorf("Expected MFAVerifyPage HTML to contain %q, but not found", snippet)
		}
	}
}

func TestAuthPasswordRecoveryPages_Render(t *testing.T) {
	ctx := context.Background()

	// Forgot Password
	var bufForgot bytes.Buffer
	compForgot := pages.ForgotPasswordPage("ar", "rtl")
	if err := compForgot.Render(ctx, &bufForgot); err != nil {
		t.Fatalf("ForgotPasswordPage.Render failed: %v", err)
	}
	htmlForgot := bufForgot.String()
	if !strings.Contains(htmlForgot, "auth-page-wrapper") || !strings.Contains(htmlForgot, "استعادة كلمة المرور") {
		t.Errorf("Expected ForgotPasswordPage to render properly")
	}

	// Reset Password
	var bufReset bytes.Buffer
	compReset := pages.ResetPasswordPage("ar", "rtl", "test-token-123")
	if err := compReset.Render(ctx, &bufReset); err != nil {
		t.Fatalf("ResetPasswordPage.Render failed: %v", err)
	}
	htmlReset := bufReset.String()
	if !strings.Contains(htmlReset, "auth-page-wrapper") || !strings.Contains(htmlReset, "تعيين كلمة المرور") || !strings.Contains(htmlReset, "test-token-123") {
		t.Errorf("Expected ResetPasswordPage to render properly")
	}
}
