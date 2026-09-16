package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func TestMFA_FullLoginChallengeFlow(t *testing.T) {
	h, idSvc, repo, _ := setupTestMFAHandler(t)
	ctx := context.Background()

	// 1. Register a user
	user, _, err := idSvc.Register(ctx, identity.RegisterInput{
		Email:    "pharmacist-mfa@dawa24.eg",
		Password: "Password123!",
		NameAr:   "صيدلي تجريبي",
		NameEn:   "Test Pharmacist",
		Role:     "customer",
		Language: i18n.AR,
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 2. Normal Login when MFA is DISABLED
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader("email=pharmacist-mfa@dawa24.eg&password=Password123!"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.LoginSubmit(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 see other, got %d", res.StatusCode)
	}
	cookies := res.Cookies()
	var sessCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "dawa24_session" {
			sessCookie = c
			break
		}
	}
	if sessCookie == nil {
		t.Fatalf("expected dawa24_session cookie on login without MFA")
	}

	// 3. Setup and Enable MFA for this user
	secret, err := identity.GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}
	_ = repo.UpsertMFA(ctx, &identity.UserMFA{
		UserID:     user.ID,
		TOTPSecret: []byte(secret),
	})
	validCode, _ := identity.GenerateTOTPCode(secret, time.Now().UTC())
	actResult, err := idSvc.ConfirmEnableMFA(ctx, user.ID, validCode)
	if err != nil || !actResult.Success {
		t.Fatalf("ConfirmEnableMFA failed: %v", err)
	}

	// 4. Login when MFA is ENABLED -> MUST redirect to /auth/mfa-verify and issue pending cookie
	reqMFA := httptest.NewRequest("POST", "/auth/login", strings.NewReader("email=pharmacist-mfa@dawa24.eg&password=Password123!"))
	reqMFA.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recMFA := httptest.NewRecorder()
	h.LoginSubmit(recMFA, reqMFA)

	resMFA := recMFA.Result()
	loc := resMFA.Header.Get("Location")
	if !strings.HasPrefix(loc, "/auth/mfa-verify") {
		t.Fatalf("expected redirect to /auth/mfa-verify, got %s", loc)
	}

	var pendingCookie *http.Cookie
	for _, c := range resMFA.Cookies() {
		if c.Name == "dawa24_mfa_pending" {
			pendingCookie = c
			break
		}
	}
	if pendingCookie == nil || pendingCookie.Value == "" {
		t.Fatalf("expected dawa24_mfa_pending cookie issued")
	}

	// 5. Test MFA Verify with WRONG code
	reqWrong := httptest.NewRequest("POST", "/auth/mfa-verify", strings.NewReader("code=000000"))
	reqWrong.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqWrong.AddCookie(pendingCookie)
	recWrong := httptest.NewRecorder()
	h.MFAVerifySubmit(recWrong, reqWrong)

	resWrong := recWrong.Result()
	if !strings.Contains(resWrong.Header.Get("Location"), "error=invalid_code") {
		t.Fatalf("expected invalid_code redirect on wrong code, got %s", resWrong.Header.Get("Location"))
	}

	// 6. Test MFA Verify with VALID TOTP code
	freshCode, _ := identity.GenerateTOTPCode(secret, time.Now().UTC())
	reqValid := httptest.NewRequest("POST", "/auth/mfa-verify", strings.NewReader("code="+freshCode))
	reqValid.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqValid.AddCookie(pendingCookie)
	recValid := httptest.NewRecorder()
	h.MFAVerifySubmit(recValid, reqValid)

	resValid := recValid.Result()
	if resValid.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 see other, got %d", resValid.StatusCode)
	}

	var finalSessCookie *http.Cookie
	var clearedPendingCookie *http.Cookie
	for _, c := range resValid.Cookies() {
		if c.Name == "dawa24_session" {
			finalSessCookie = c
		}
		if c.Name == "dawa24_mfa_pending" {
			clearedPendingCookie = c
		}
	}
	if finalSessCookie == nil || finalSessCookie.Value == "" {
		t.Fatalf("expected dawa24_session cookie after successful MFA verification")
	}
	if clearedPendingCookie == nil || clearedPendingCookie.MaxAge != -1 {
		t.Fatalf("expected dawa24_mfa_pending cookie to be cleared (MaxAge=-1)")
	}

	// 7. Test MFA Verify with single-use RECOVERY CODE
	reqMFA2 := httptest.NewRequest("POST", "/auth/login", strings.NewReader("email=pharmacist-mfa@dawa24.eg&password=Password123!"))
	reqMFA2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recMFA2 := httptest.NewRecorder()
	h.LoginSubmit(recMFA2, reqMFA2)

	var pendingCookie2 *http.Cookie
	for _, c := range recMFA2.Result().Cookies() {
		if c.Name == "dawa24_mfa_pending" {
			pendingCookie2 = c
			break
		}
	}

	recoveryCode := actResult.RecoveryCodes[0]
	form := url.Values{}
	form.Set("recovery_code", recoveryCode)
	reqRec := httptest.NewRequest("POST", "/auth/mfa-verify", strings.NewReader(form.Encode()))
	reqRec.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqRec.AddCookie(pendingCookie2)
	recRec := httptest.NewRecorder()
	h.MFAVerifySubmit(recRec, reqRec)

	if recRec.Result().StatusCode != http.StatusSeeOther {
		t.Fatalf("expected login success via recovery code, got %d", recRec.Result().StatusCode)
	}

	// Reusing same recovery code should now FAIL
	reqMFA3 := httptest.NewRequest("POST", "/auth/login", strings.NewReader("email=pharmacist-mfa@dawa24.eg&password=Password123!"))
	reqMFA3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recMFA3 := httptest.NewRecorder()
	h.LoginSubmit(recMFA3, reqMFA3)

	var pendingCookie3 *http.Cookie
	for _, c := range recMFA3.Result().Cookies() {
		if c.Name == "dawa24_mfa_pending" {
			pendingCookie3 = c
			break
		}
	}

	reqRec2 := httptest.NewRequest("POST", "/auth/mfa-verify", strings.NewReader(form.Encode()))
	reqRec2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqRec2.AddCookie(pendingCookie3)
	recRec2 := httptest.NewRecorder()
	h.MFAVerifySubmit(recRec2, reqRec2)

	if !strings.Contains(recRec2.Result().Header.Get("Location"), "error=invalid_code") {
		t.Fatalf("expected reuse of consumed recovery code to fail")
	}
}

func TestMFA_DashboardSettingsHandlers(t *testing.T) {
	h, idSvc, _, _ := setupTestMFAHandler(t)
	ctx := context.Background()

	// Register user
	user, _, err := idSvc.Register(ctx, identity.RegisterInput{
		Email:    "vendor-mfa@dawa24.eg",
		Password: "VendorPassword123!",
		NameAr:   "مورد تجريبي",
		NameEn:   "Test Vendor",
		Role:     "vendor",
		Language: i18n.AR,
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	actor := authctx.Actor{
		UserID:         user.ID,
		Email:          user.Email,
		Role:           "vendor",
		OrganizationID: 20,
		OrgType:        "vendor",
		Permissions:    []string{"vendor.session.view"},
	}
	ctxWithActor := authctx.WithActor(context.Background(), actor)

	// 1. GET /vendor/mfa (Unenrolled state)
	reqGet := httptest.NewRequest("GET", "/vendor/mfa", nil).WithContext(ctxWithActor)
	recGet := httptest.NewRecorder()
	h.VendorMFAPage(recGet, reqGet)
	if recGet.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on /vendor/mfa, got %d", recGet.Result().StatusCode)
	}

	// 2. POST /vendor/mfa/setup (Initiate setup)
	reqSetup := httptest.NewRequest("POST", "/vendor/mfa/setup", nil).WithContext(ctxWithActor)
	recSetup := httptest.NewRecorder()
	h.VendorMFASetupSubmit(recSetup, reqSetup)
	if recSetup.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on setup, got %d", recSetup.Result().StatusCode)
	}

	// 3. Disable MFA
	reqDisable := httptest.NewRequest("POST", "/vendor/mfa/disable", strings.NewReader("password=VendorPassword123!")).WithContext(ctxWithActor)
	reqDisable.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recDisable := httptest.NewRecorder()
	h.VendorMFADisableSubmit(recDisable, reqDisable)
	if recDisable.Result().StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 see other on disable, got %d", recDisable.Result().StatusCode)
	}

	mfaAfter, _ := idSvc.GetMFAStatus(ctx, user.ID)
	if mfaAfter.Enabled {
		t.Errorf("expected MFA to be disabled after disable submit")
	}
}

func TestSessionRevocation_PermissionsEnforcement(t *testing.T) {
	h, idSvc, _, store := setupTestMFAHandler(t)
	ctx := context.Background()

	// User 1 (Regular Member)
	u1, _, _ := idSvc.Register(ctx, identity.RegisterInput{Email: "emp1@org.eg", Password: "Password123!", NameAr: "موظف 1", Role: "user"})
	// User 2 (Regular Member)
	u2, _, _ := idSvc.Register(ctx, identity.RegisterInput{Email: "emp2@org.eg", Password: "Password123!", NameAr: "موظف 2", Role: "user"})

	// Create sessions in store
	s1 := &identity.Session{Token: "token-user1", UserID: u1.ID, ActiveOrgID: 10}
	s2 := &identity.Session{Token: "token-user2", UserID: u2.ID, ActiveOrgID: 10}
	_ = store.Create(ctx, s1)
	_ = store.Create(ctx, s2)

	// Case 1: User 1 attempts to revoke User 2's session (NOT super admin) -> MUST BE REJECTED
	actorU1 := authctx.Actor{
		UserID:         u1.ID,
		Email:          u1.Email,
		Role:           "user",
		OrganizationID: 10,
		OrgType:        "customer",
	}
	ctxU1 := authctx.WithActor(context.Background(), actorU1)

	reqRevokeU2 := httptest.NewRequest("POST", "/customer/sessions/revoke", strings.NewReader("token=token-user2")).WithContext(ctxU1)
	reqRevokeU2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recRevokeU2 := httptest.NewRecorder()
	h.TenantSessionRevokeSubmit(recRevokeU2, reqRevokeU2)

	loc := recRevokeU2.Result().Header.Get("Location")
	if !strings.Contains(loc, "notice=error") {
		t.Fatalf("expected error notice when regular user tries to revoke another user's session, got %s", loc)
	}

	// Session 2 should still be alive in store
	if sess2, err := store.Get(ctx, "token-user2"); err != nil || sess2 == nil {
		t.Errorf("user 2 session should NOT have been revoked")
	}

	// Case 2: Super Admin attempts to revoke User 2's session -> ALLOWED
	actorSuper := authctx.Actor{
		UserID:         999,
		Email:          "superadmin@dawa24.eg",
		Role:           "super_admin",
		OrganizationID: 10,
		OrgType:        "customer",
	}
	ctxSuper := authctx.WithActor(context.Background(), actorSuper)

	reqRevokeSuper := httptest.NewRequest("POST", "/customer/sessions/revoke", strings.NewReader("token=token-user2")).WithContext(ctxSuper)
	reqRevokeSuper.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recRevokeSuper := httptest.NewRecorder()
	h.TenantSessionRevokeSubmit(recRevokeSuper, reqRevokeSuper)

	locSuper := recRevokeSuper.Result().Header.Get("Location")
	if strings.Contains(locSuper, "notice=error") {
		t.Fatalf("super admin should be allowed to revoke any session in the org, got error %s", locSuper)
	}
}
