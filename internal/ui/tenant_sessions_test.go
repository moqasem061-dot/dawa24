package ui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestTenantSessionsPage_RendersAndManagesSessions(t *testing.T) {
	db := testDB(t)
	if db == nil {
		t.Skip("database not available")
	}

	handler := newRealUIHandler(t, db)
	ctx := context.Background()

	// 1. Create a customer org and user
	var orgID int64
	err := db.Pool().QueryRow(ctx, `
		INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ('{"ar":"صيدلية إدارة الجلسات"}', 'صيدلية إدارة الجلسات', '{"ar":"صيدلية إدارة الجلسات"}', 'TAX-SESS-101', 'CR-SESS-101', 'customer', 'approved')
		RETURNING id
	`).Scan(&orgID)
	if err != nil {
		t.Fatalf("failed to insert test customer org: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM org.organizations WHERE id = $1`, orgID)
	}()

	var userID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO identity.users (email, password_hash, name, role, status)
		VALUES ('sess_customer@dawa24.eg', '$2a$10$abcdefghijklmnopqrstuu', '{"ar":"د. جلسات"}', 'user', 'active')
		RETURNING id
	`).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM identity.users WHERE id = $1`, userID)
	}()

	_, _ = db.Pool().Exec(ctx, `
		INSERT INTO org.members (organization_id, user_id, role_key, status)
		VALUES ($1, $2, 'owner', 'active')
	`, orgID, userID)

	// 2. Test GET /customer/sessions
	req := httptest.NewRequest("GET", "/customer/sessions", nil)
	customerActor := authctx.Actor{
		UserID:         userID,
		OrganizationID: orgID,
		OrgType:        "customer",
		Role:           "owner",
		IsOwner:        true,
		Permissions:    []string{"pharmacy.session.view", "pharmacy.session.manage", "pharmacy.session.revoke"},
	}
	req = req.WithContext(authctx.WithActor(req.Context(), customerActor))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /customer/sessions returned %d, expected 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "الأجهزة والجلسات النشطة") {
		t.Errorf("expected page title in body, got: %s", body)
	}
	if !strings.Contains(body, "الجلسات المتصلة") {
		t.Errorf("expected sessions quota card in body")
	}

	// 3. Test GET /vendor/sessions
	var vendorOrgID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ('{"ar":"شركة التوريد والجلسات"}', 'شركة التوريد والجلسات', '{"ar":"شركة التوريد والجلسات"}', 'TAX-VSESS-102', 'CR-VSESS-102', 'vendor', 'approved')
		RETURNING id
	`).Scan(&vendorOrgID)
	if err != nil {
		t.Fatalf("failed to insert test vendor org: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM org.organizations WHERE id = $1`, vendorOrgID)
	}()

	vReq := httptest.NewRequest("GET", "/vendor/sessions", nil)
	vendorActor := authctx.Actor{
		UserID:         userID,
		OrganizationID: vendorOrgID,
		OrgType:        "vendor",
		Role:           "owner",
		IsOwner:        true,
		Permissions:    []string{"vendor.session.view", "vendor.session.manage", "vendor.session.revoke"},
	}
	vReq = vReq.WithContext(authctx.WithActor(vReq.Context(), vendorActor))
	vRec := httptest.NewRecorder()
	handler.ServeHTTP(vRec, vReq)

	if vRec.Code != http.StatusOK {
		t.Fatalf("GET /vendor/sessions returned %d, expected 200", vRec.Code)
	}

	// 4. Test POST /customer/sessions/revoke
	revokeForm := url.Values{}
	revokeForm.Set("token", "dummy_token_to_revoke")
	revokeReq := httptest.NewRequest("POST", "/customer/sessions/revoke", strings.NewReader(revokeForm.Encode()))
	revokeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	revokeReq = revokeReq.WithContext(authctx.WithActor(revokeReq.Context(), customerActor))
	revokeRec := httptest.NewRecorder()
	handler.ServeHTTP(revokeRec, revokeReq)

	if revokeRec.Code != http.StatusSeeOther {
		t.Fatalf("POST /customer/sessions/revoke returned %d, expected 303 redirect", revokeRec.Code)
	}
	if !strings.Contains(revokeRec.Header().Get("Location"), "/customer/sessions") {
		t.Errorf("expected redirect to /customer/sessions, got %s", revokeRec.Header().Get("Location"))
	}

	// 5. Test POST /customer/sessions/revoke-all
	revokeAllForm := url.Values{}
	revokeAllForm.Set("current_token", "active_current_token")
	revokeAllReq := httptest.NewRequest("POST", "/customer/sessions/revoke-all", strings.NewReader(revokeAllForm.Encode()))
	revokeAllReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	revokeAllReq = revokeAllReq.WithContext(authctx.WithActor(revokeAllReq.Context(), customerActor))
	revokeAllRec := httptest.NewRecorder()
	handler.ServeHTTP(revokeAllRec, revokeAllReq)

	if revokeAllRec.Code != http.StatusSeeOther {
		t.Fatalf("POST /customer/sessions/revoke-all returned %d, expected 303 redirect", revokeAllRec.Code)
	}

	// 6. Test GET /auth/login?error=concurrent_limit
	loginReq := httptest.NewRequest("GET", "/auth/login?error=concurrent_limit", nil)
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("GET /auth/login?error=concurrent_limit returned %d, expected 200", loginRec.Code)
	}
	loginBody := loginRec.Body.String()
	if !strings.Contains(loginBody, "تم إنهاء جلستك تلقائياً نظراً لتسجيل الدخول من جهاز آخر") {
		t.Errorf("expected concurrent session eviction explanation in login body")
	}
	if !strings.Contains(loginBody, "تذكرني على هذا الجهاز") {
		t.Errorf("expected 'Remember Me' checkbox in login body")
	}

	// 7. Test GET /auth/login?error=duplicate_login
	dupLoginReq := httptest.NewRequest("GET", "/auth/login?error=duplicate_login", nil)
	dupLoginRec := httptest.NewRecorder()
	handler.ServeHTTP(dupLoginRec, dupLoginReq)

	if dupLoginRec.Code != http.StatusOK {
		t.Fatalf("GET /auth/login?error=duplicate_login returned %d, expected 200", dupLoginRec.Code)
	}
	dupLoginBody := dupLoginRec.Body.String()
	if !strings.Contains(dupLoginBody, "تم تسجيل خروجك تلقائياً نظراً لتسجيل الدخول إلى حسابك من جهاز أو متصفح آخر") {
		t.Errorf("expected duplicate login eviction explanation in login body")
	}
}

func TestTenantSessions_SingularRedirects(t *testing.T) {
	h := &ui.UIHandler{}
	router := newRealUIHandlerRouter(h)

	// 1. Customer singular redirect
	req := httptest.NewRequest("GET", "/customer/session", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /customer/session returned %d, expected 301", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/customer/sessions" {
		t.Errorf("expected redirect to /customer/sessions, got %s", loc)
	}

	// 2. Vendor singular redirect
	vReq := httptest.NewRequest("GET", "/vendor/session", nil)
	vRec := httptest.NewRecorder()
	router.ServeHTTP(vRec, vReq)

	if vRec.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /vendor/session returned %d, expected 301", vRec.Code)
	}
	if loc := vRec.Header().Get("Location"); loc != "/vendor/sessions" {
		t.Errorf("expected redirect to /vendor/sessions, got %s", loc)
	}
}
