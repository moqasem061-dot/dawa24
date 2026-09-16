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
)

func TestTenantWallet_PharmacyAndVendor_E2E(t *testing.T) {
	db := testDB(t)
	if db == nil {
		t.Skip("database not available")
	}

	handler := newRealUIHandler(t, db)
	ctx := context.Background()

	// 1. Create a customer organization and user
	var customerOrgID int64
	err := db.Pool().QueryRow(ctx, `
		INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ('{"ar":"صيدلية المحفظة التجريبية"}', 'صيدلية المحفظة التجريبية', '{"ar":"صيدلية المحفظة التجريبية"}', 'TAX-WAL-101', 'CR-WAL-101', 'customer', 'approved')
		RETURNING id
	`).Scan(&customerOrgID)
	if err != nil {
		t.Fatalf("failed to insert customer org: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM org.organizations WHERE id = $1`, customerOrgID)
	}()

	var userID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO identity.users (name, email, password_hash, role, status)
		VALUES ('{"ar":"دكتور المحفظة"}', 'wallet_test@dawa24.eg', '$2a$10$abcdefghijklmnopqrstuu', 'user', 'active')
		RETURNING id
	`).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM identity.users WHERE id = $1`, userID)
	}()

	// 2. Test GET /customer/wallet
	cReq := httptest.NewRequest("GET", "/customer/wallet", nil)
	customerActor := authctx.Actor{
		UserID:         userID,
		OrganizationID: customerOrgID,
		OrgType:        "customer",
		Role:           "owner",
		IsOwner:        true,
		Permissions:    []string{"pharmacy.wallet.view", "pharmacy.wallet.manage"},
	}
	cReq = cReq.WithContext(authctx.WithActor(cReq.Context(), customerActor))
	cRec := httptest.NewRecorder()
	handler.ServeHTTP(cRec, cReq)

	if cRec.Code != http.StatusOK {
		t.Fatalf("GET /customer/wallet returned %d, expected 200", cRec.Code)
	}
	cBody := cRec.Body.String()
	if !strings.Contains(cBody, "المحفظة والرصيد") {
		t.Errorf("expected wallet page title in customer body")
	}
	if !strings.Contains(cBody, "الرصيد المتاح") {
		t.Errorf("expected available balance section in customer body")
	}

	// 3. Test POST /customer/wallet/payment-methods (Add Instapay)
	form := url.Values{
		"provider":        {"instapay"},
		"instapay_handle": {"pharmacy@instapay"},
		"is_default":      {"1"},
	}
	addReq := httptest.NewRequest("POST", "/customer/wallet/payment-methods", strings.NewReader(form.Encode()))
	addReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addReq = addReq.WithContext(authctx.WithActor(addReq.Context(), customerActor))
	addRec := httptest.NewRecorder()
	handler.ServeHTTP(addRec, addReq)

	if addRec.Code != http.StatusSeeOther {
		t.Fatalf("POST /customer/wallet/payment-methods returned %d, expected 303 redirect", addRec.Code)
	}
	if loc := addRec.Header().Get("Location"); !strings.Contains(loc, "/customer/wallet") {
		t.Errorf("expected redirect to /customer/wallet, got: %s", loc)
	}

	// 4. Test Vendor GET /vendor/wallet
	var vendorOrgID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ('{"ar":"شركة التوريد المحفظية"}', 'شركة التوريد المحفظية', '{"ar":"شركة التوريد المحفظية"}', 'TAX-VWAL-102', 'CR-VWAL-102', 'vendor', 'approved')
		RETURNING id
	`).Scan(&vendorOrgID)
	if err != nil {
		t.Fatalf("failed to insert vendor org: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM org.organizations WHERE id = $1`, vendorOrgID)
	}()

	vReq := httptest.NewRequest("GET", "/vendor/wallet", nil)
	vendorActor := authctx.Actor{
		UserID:         userID,
		OrganizationID: vendorOrgID,
		OrgType:        "vendor",
		Role:           "owner",
		IsOwner:        true,
		Permissions:    []string{"vendor.wallet.view", "vendor.wallet.manage"},
	}
	vReq = vReq.WithContext(authctx.WithActor(vReq.Context(), vendorActor))
	vRec := httptest.NewRecorder()
	handler.ServeHTTP(vRec, vReq)

	if vRec.Code != http.StatusOK {
		t.Fatalf("GET /vendor/wallet returned %d, expected 200", vRec.Code)
	}
	vBody := vRec.Body.String()
	if !strings.Contains(vBody, "المحفظة والرصيد") {
		t.Errorf("expected wallet page title in vendor body")
	}

	// 5. Test Generic /wallet 301 redirection
	genReq := httptest.NewRequest("GET", "/wallet", nil)
	genReq = genReq.WithContext(authctx.WithActor(genReq.Context(), vendorActor))
	genRec := httptest.NewRecorder()
	handler.ServeHTTP(genRec, genReq)

	if genRec.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /wallet returned %d, expected 301", genRec.Code)
	}
	if loc := genRec.Header().Get("Location"); loc != "/vendor/wallet" {
		t.Errorf("expected /wallet redirect to /vendor/wallet for vendor, got: %s", loc)
	}
}
