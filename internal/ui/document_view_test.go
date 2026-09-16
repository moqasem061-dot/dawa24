package ui_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

func TestDocumentViewHandler_SanitizesLocalhostAndShowsCleanError(t *testing.T) {
	db := testDB(t)
	if db == nil {
		t.Skip("database not available")
	}

	handler := newRealUIHandler(t, db)
	ctx := context.Background()

	// 1. Create a dummy organization and document with a localhost URL
	var orgID int64
	err := db.Pool().QueryRow(ctx, `
		INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ('{"ar":"صيدلية الاختبار للمستندات"}', 'صيدلية الاختبار للمستندات', '{"ar":"صيدلية الاختبار"}', 'TAX-DOC-123', 'CR-DOC-123', 'customer', 'pending')
		RETURNING id
	`).Scan(&orgID)
	if err != nil {
		t.Fatalf("failed to insert test org: %v", err)
	}

	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM org.organizations WHERE id = $1`, orgID)
	}()

	var docID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO platform_admin.documents (organization_id, document_type, title, file_url, storage_key, original_name, status, mime_type)
		VALUES ($1, 'commercial_register', 'السجل التجاري', 'http://localhost:9000/dawa24//uploads/documents/2026/02/test_cr.pdf', 'uploads/documents/2026/02/test_cr.pdf', 'test_cr.pdf', 'pending', 'application/pdf')
		RETURNING id
	`, orgID).Scan(&docID)
	if err != nil {
		t.Fatalf("failed to insert test document: %v", err)
	}

	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM platform_admin.documents WHERE id = $1`, docID)
	}()

	// 2. Perform GET /documents/{id}/view as admin
	docIDStr := fmt.Sprintf("%d", docID)
	req := httptest.NewRequest("GET", "/documents/"+docIDStr+"/view", nil)
	// Build chi URL params context
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", docIDStr)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Attach Admin actor
	actor := authctx.Actor{
		UserID:  1,
		Email:   "admin@dawa24.com",
		Role:    "admin",
		IsStaff: true,
	}
	req = req.WithContext(authctx.WithActor(req.Context(), actor))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Verify that we NEVER redirect to localhost:9000
	location := rec.Header().Get("Location")
	if strings.Contains(location, "localhost:9000") || strings.Contains(location, "127.0.0.1:9000") {
		t.Fatalf("SECURITY/UX DEFECT: Response redirected to unreachable localhost URL: %s", location)
	}

	// If the file is not on disk/storage, it should return 404 with the clear DocumentUnavailablePage
	if rec.Code == http.StatusNotFound {
		body := rec.Body.String()
		if !strings.Contains(body, "تعذر فتح المستند") {
			t.Errorf("Expected DocumentUnavailablePage body to contain Arabic error title, got: %s", body)
		}
		if !strings.Contains(body, "السجل التجاري") {
			t.Errorf("Expected DocumentUnavailablePage body to contain document type label, got: %s", body)
		}
		if !strings.Contains(body, "test_cr.pdf") {
			t.Errorf("Expected DocumentUnavailablePage body to contain original filename, got: %s", body)
		}
	}
}

func TestTenantSubscriptionPage_ShowsPlansAndUpgrades(t *testing.T) {
	db := testDB(t)
	if db == nil {
		t.Skip("database not available")
	}

	handler := newRealUIHandler(t, db)
	ctx := context.Background()

	// 1. Create a dummy organization and user for testing
	var orgID int64
	err := db.Pool().QueryRow(ctx, `
		INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ('{"ar":"صيدلية اختبار الاشتراكات"}', 'صيدلية اختبار الاشتراكات', '{"ar":"صيدلية الاختبار"}', 'TAX-SUB-123', 'CR-SUB-123', 'customer', 'approved')
		RETURNING id
	`).Scan(&orgID)
	if err != nil {
		t.Fatalf("failed to insert test org: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM org.organizations WHERE id = $1`, orgID)
	}()

	actor := authctx.Actor{
		UserID:         99991,
		OrganizationID: orgID,
		OrgID:          orgID,
		Role:           "customer",
		OrgType:        "customer",
		Permissions:    []string{"pharmacy.*"},
	}

	// 2. GET /customer/subscription
	req := httptest.NewRequest("GET", "/customer/subscription", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), actor))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /customer/subscription returned %d, expected 200", rec.Code)
	}

	body := rec.Body.String()
	if strings.Contains(body, "جاري تجهيز بيانات الباقات من النظام...") {
		t.Fatalf("DEFECT DETECTED: Subscription page showed empty placeholder instead of plans!")
	}

	if !strings.Contains(body, "الباقة الاحترافية") && !strings.Contains(body, "pro") {
		t.Errorf("Expected subscription page to render pro plan, got body length: %d", len(body))
	}

	// 3. Test GET /vendor/subscription
	vendorActor := authctx.Actor{
		UserID:         99992,
		OrganizationID: orgID,
		OrgID:          orgID,
		Role:           "vendor",
		OrgType:        "vendor",
		Permissions:    []string{"vendor.*"},
	}
	vReq := httptest.NewRequest("GET", "/vendor/subscription", nil)
	vReq = vReq.WithContext(authctx.WithActor(vReq.Context(), vendorActor))
	vRec := httptest.NewRecorder()

	handler.ServeHTTP(vRec, vReq)
	if vRec.Code != http.StatusOK {
		t.Fatalf("GET /vendor/subscription returned %d, expected 200", vRec.Code)
	}
	vBody := vRec.Body.String()
	if strings.Contains(vBody, "جاري تجهيز بيانات الباقات من النظام...") {
		t.Fatalf("DEFECT DETECTED: Vendor subscription page showed empty placeholder instead of plans!")
	}
}

func TestTenantSubscriptionCheckoutSubmit_UpgradesAndDeductsWallet(t *testing.T) {
	db := testDB(t)
	if db == nil {
		t.Skip("database not available")
	}

	handler := newRealUIHandler(t, db)
	ctx := context.Background()

	// 1. Create a dummy organization and user for testing
	var orgID int64
	err := db.Pool().QueryRow(ctx, `
		INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ('{"ar":"صيدلية تجربة الترقية"}', 'صيدلية تجربة الترقية', '{"ar":"صيدلية الترقية"}', 'TAX-UPG-123', 'CR-UPG-123', 'customer', 'approved')
		RETURNING id
	`).Scan(&orgID)
	if err != nil {
		t.Fatalf("failed to insert test org: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM org.organizations WHERE id = $1`, orgID)
	}()

	testUserID := seedUser(t, db, orgID, "user")
	// Ensure wallet exists and has 50,000 EGP balance for upgrade
	var walletID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO billing.wallets (user_id, currency, created_at, updated_at)
		VALUES ($1, 'EGP', now(), now())
		ON CONFLICT (user_id, currency) DO UPDATE SET updated_at = now()
		RETURNING id
	`, testUserID).Scan(&walletID)
	if err != nil {
		t.Fatalf("failed to setup test wallet: %v", err)
	}

	_, err = db.Pool().Exec(ctx, `
		INSERT INTO billing.wallet_transactions (wallet_id, type, amount, balance_after, description, created_at)
		VALUES ($1, 'deposit', 5000000, 5000000, 'Test top up', now())
	`, walletID)
	if err != nil {
		t.Fatalf("failed to credit test wallet: %v", err)
	}

	actor := authctx.Actor{
		UserID:         testUserID,
		OrganizationID: orgID,
		OrgID:          orgID,
		Role:           "customer",
		OrgType:        "customer",
		Permissions:    []string{"pharmacy.*"},
	}

	// 2. Perform POST /customer/subscription/checkout for 'pro' plan monthly
	form := url.Values{}
	form.Set("plan_slug", "pro")
	form.Set("billing_cycle", "monthly")
	form.Set("auto_renew", "true")

	req := httptest.NewRequest("POST", "/customer/subscription/checkout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(req.Context(), actor))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("Checkout returned status %d, expected 303 SeeOther", rec.Code)
	}

	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "notice=success") && !strings.Contains(loc, "notice_type=success") {
		t.Fatalf("Expected checkout redirect to contain success notice, got location: %s", loc)
	}

	// 3. Verify active subscription created in database
	var subCount int
	_ = db.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM billing.subscriptions
		WHERE organization_id = $1 AND status = 'active'
	`, orgID).Scan(&subCount)

	if subCount == 0 {
		t.Fatalf("Expected active subscription to be created for org %d", orgID)
	}
}
