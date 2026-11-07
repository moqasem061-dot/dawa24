package ui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

func TestVendorPaymentsLifecycle_E2E(t *testing.T) {
	db := testDB(t)
	if db == nil {
		t.Skip("database not available")
	}

	seedVendorPaymentsFixture(t, db)

	handler := newRealUIHandler(t, db)

	vendorActor := authctx.Actor{
		UserID:         14,
		OrganizationID: 51,
		OrgType:        "supplier",
		Role:           "vendor",
		Permissions:    []string{"vendor.*"},
	}

	// 1. GET /vendor/payments should return 200 OK and show open invoices
	reqGet := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/vendor/payments", nil)
	reqGet = reqGet.WithContext(authctx.WithActor(reqGet.Context(), vendorActor))
	recGet := httptest.NewRecorder()
	handler.ServeHTTP(recGet, reqGet)

	if recGet.Code != http.StatusOK {
		t.Fatalf("GET /vendor/payments failed: expected 200, got %d", recGet.Code)
	}
	bodyGet := recGet.Body.String()
	if !strings.Contains(bodyGet, "INV-2026-00028") {
		t.Errorf("Expected invoice INV-2026-00028 to be present in modal or page")
	}

	// 2. POST /vendor/payments/record to record a partial payment of 50.00 EGP on invoice 21
	form := url.Values{}
	form.Set("invoice_id", "21")
	form.Set("amount", "50.00")
	form.Set("method", "cash")
	form.Set("reference_number", "TRX-E2E-PARTIAL-50")
	form.Set("notes", "Partial payment via automated E2E test")

	reqPost := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/vendor/payments/record", strings.NewReader(form.Encode()))
	reqPost.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqPost = reqPost.WithContext(authctx.WithActor(reqPost.Context(), vendorActor))
	recPost := httptest.NewRecorder()
	handler.ServeHTTP(recPost, reqPost)

	if recPost.Code != http.StatusSeeOther {
		t.Fatalf("POST /vendor/payments/record failed: expected 303 redirect, got %d: %s", recPost.Code, recPost.Body.String())
	}
	loc := recPost.Header().Get("Location")
	if strings.Contains(loc, "error") {
		t.Fatalf("POST /vendor/payments/record redirected with error: %s", loc)
	}

	// 3. Verify in database that invoice 21 is now partially_paid
	var status string
	err := db.Pool().QueryRow(context.Background(), `SELECT status FROM billing.invoices WHERE id = 21`).Scan(&status)
	if err != nil {
		t.Fatalf("Failed to query invoice status: %v", err)
	}
	if status != "partially_paid" {
		t.Errorf("Expected invoice 21 to be 'partially_paid', got '%s'", status)
	}

	// 4. Verify that GET /vendor/payments now lists this payment in the table!
	recGetAfter := httptest.NewRecorder()
	handler.ServeHTTP(recGetAfter, reqGet)
	bodyGetAfter := recGetAfter.Body.String()
	if !strings.Contains(bodyGetAfter, "TRX-E2E-PARTIAL-50") {
		t.Errorf("Expected table to display transaction TRX-E2E-PARTIAL-50, body length=%d", len(bodyGetAfter))
	}
	if !strings.Contains(bodyGetAfter, "50.00") {
		t.Errorf("Expected table to display amount 50.00")
	}

	// 5. Clean up created test payment and reset invoice status
	_, _ = db.Pool().Exec(database.AsSystem(context.Background()),
		`DELETE FROM billing.payments WHERE reference_number = 'TRX-E2E-PARTIAL-50'`)
	_, _ = db.Pool().Exec(database.AsSystem(context.Background()),
		`UPDATE billing.invoices SET status = 'issued', updated_at = now() WHERE id = 21`)
}

// seedVendorPaymentsFixture creates the organization, user and open invoice the
// test names. They used to be legacy rows (org 51, user 14, invoice 21 numbered
// INV-2026-00028), which a fresh database does not have. The test resets the
// invoice to issued when it finishes, so the fixture is safe to upsert.
func seedVendorPaymentsFixture(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	stmts := []string{
		`INSERT INTO org.organizations (id, name, type, status) VALUES (51, '{"ar":"مورد الدفعات","en":"Payments Vendor"}'::jsonb, 'supplier', 'approved') ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO identity.users (id, email, password_hash, name) VALUES (14, 'vendor-payments-14@example.com', 'x', '{"ar":"","en":""}') ON CONFLICT DO NOTHING`,
		`INSERT INTO billing.invoices (id, organization_id, invoice_number, due_date, total_amount, status)
		 VALUES (21, 51, 'INV-2026-00028', CURRENT_DATE + 30, 500.00, 'issued')
		 ON CONFLICT (id) DO UPDATE SET status = 'issued', updated_at = now()`,
	}
	for _, s := range stmts {
		if _, err := db.Pool().Exec(ctx, s); err != nil {
			t.Fatalf("seed vendor payments fixture: %v", err)
		}
	}
}
