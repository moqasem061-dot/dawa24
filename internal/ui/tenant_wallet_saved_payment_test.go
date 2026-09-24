package ui_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

func TestTenantWallet_SavedPaymentMethods_AutoWiring_E2E(t *testing.T) {
	db := testDB(t)
	if db == nil {
		t.Skip("database not available")
	}

	handler := newRealUIHandler(t, db)
	ctx := context.Background()

	// 1. Create test user and organization
	var orgID int64
	err := db.Pool().QueryRow(ctx, `
		INSERT INTO org.organizations (name, legal_name, trade_name, tax_number, commercial_register, type, status)
		VALUES ('{"ar":"صيدلية ربط الدفع"}', 'صيدلية ربط الدفع', '{"ar":"صيدلية ربط الدفع"}', 'TAX-AUTO-301', 'CR-AUTO-301', 'customer', 'approved')
		RETURNING id
	`).Scan(&orgID)
	if err != nil {
		t.Fatalf("failed to insert customer org: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM org.organizations WHERE id = $1`, orgID)
	}()

	var userID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO identity.users (name, email, password_hash, role, status)
		VALUES ('{"ar":"دكتور الاختبار"}', 'auto_wiring_test@dawa24.eg', '$2a$10$abcdefghijklmnopqrstuu', 'user', 'active')
		RETURNING id
	`).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM identity.users WHERE id = $1`, userID)
	}()

	actor := authctx.Actor{
		UserID:         userID,
		OrganizationID: orgID,
		OrgType:        "customer",
		Role:           "owner",
		IsOwner:        true,
		Permissions:    []string{"pharmacy.wallet.view", "pharmacy.wallet.manage"},
	}

	// 2. Insert saved user payment method (InstaPay)
	var pmID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO billing.user_payment_methods (user_id, provider, account_identifier, is_default, details)
		VALUES ($1, 'instapay', 'InstaPay: user@instapay • Doctor Name', true, '{"instapay_handle":"user@instapay"}')
		RETURNING id
	`, userID).Scan(&pmID)
	if err != nil {
		t.Fatalf("failed to insert user payment method: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM billing.user_payment_methods WHERE id = $1`, pmID)
	}()

	// 3. Submit deposit selecting only platform_method_id and sender_payment_method_id (simulating UI dropdown)
	depForm := url.Values{
		"amount":                   {"800.00"},
		"platform_method_id":       {"instapay"},
		"sender_payment_method_id": {fmt.Sprintf("%d", pmID)},
		"reference_number":         {"REF-AUTO-9988"},
		"notes":                    {"إيداع مع وسيلة دفع مسجلة"},
	}
	depReq := httptest.NewRequestWithContext(context.Background(), "POST", "/customer/wallet/deposit", strings.NewReader(depForm.Encode()))
	depReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	depReq = depReq.WithContext(authctx.WithActor(depReq.Context(), actor))
	depRec := httptest.NewRecorder()
	handler.ServeHTTP(depRec, depReq)

	if depRec.Code != http.StatusSeeOther {
		t.Fatalf("deposit returned %d, expected 303", depRec.Code)
	}

	// Check DB columns for deposit
	var depPMID *int64
	var depSenderAcc, depPlatID string
	err = db.Pool().QueryRow(ctx, `
		SELECT sender_payment_method_id, sender_account, platform_method_id
		FROM billing.wallet_deposits
		WHERE user_id = $1
		ORDER BY id DESC LIMIT 1
	`, userID).Scan(&depPMID, &depSenderAcc, &depPlatID)
	if err != nil {
		t.Fatalf("failed to query created deposit: %v", err)
	}

	if depPMID == nil || *depPMID != pmID {
		t.Errorf("expected sender_payment_method_id %d, got %v", pmID, depPMID)
	}
	if depSenderAcc != "InstaPay: user@instapay • Doctor Name" {
		t.Errorf("expected sender_account auto-wired to saved method, got: %q", depSenderAcc)
	}
	if depPlatID != "instapay" {
		t.Errorf("expected platform_method_id 'instapay', got: %q", depPlatID)
	}

	// 4. Create wallet and credit it so balance > 0
	var walletID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO billing.wallets (user_id, currency)
		VALUES ($1, 'EGP')
		ON CONFLICT (user_id, currency) DO UPDATE SET updated_at = now()
		RETURNING id
	`, userID).Scan(&walletID)
	if err != nil {
		t.Fatalf("failed to get/create wallet: %v", err)
	}

	_, err = db.Pool().Exec(ctx, `
		INSERT INTO billing.wallet_transactions (wallet_id, type, amount, balance_after, reference_type, reference_id, description)
		VALUES ($1, 'deposit', 1000.00, 1000.00, 'deposit', 1, 'رصيد تجريبي')
	`, walletID)
	if err != nil {
		t.Fatalf("failed to insert initial transaction: %v", err)
	}

	// 5. Submit withdrawal selecting only user_payment_method_id (simulating UI dropdown)
	withForm := url.Values{
		"amount":                 {"300.00"},
		"user_payment_method_id": {fmt.Sprintf("%d", pmID)},
		"reason":                 {"سحب مع وسيلة دفع مسجلة"},
	}
	withReq := httptest.NewRequestWithContext(context.Background(), "POST", "/customer/wallet/withdraw", strings.NewReader(withForm.Encode()))
	withReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	withReq = withReq.WithContext(authctx.WithActor(withReq.Context(), actor))
	withRec := httptest.NewRecorder()
	handler.ServeHTTP(withRec, withReq)

	if withRec.Code != http.StatusSeeOther {
		t.Fatalf("withdrawal returned %d, expected 303", withRec.Code)
	}

	// Check DB columns for withdrawal
	var withUPMID *int64
	var withDest, withType, withStat string
	err = db.Pool().QueryRow(ctx, `
		SELECT user_payment_method_id, destination_details, payout_method_type, status
		FROM billing.wallet_withdrawals
		WHERE user_id = $1
		ORDER BY id DESC LIMIT 1
	`, userID).Scan(&withUPMID, &withDest, &withType, &withStat)
	if err != nil {
		t.Fatalf("failed to query created withdrawal: %v", err)
	}

	if withUPMID == nil || *withUPMID != pmID {
		t.Errorf("expected user_payment_method_id %d, got %v", pmID, withUPMID)
	}
	if withDest != "InstaPay: user@instapay • Doctor Name" {
		t.Errorf("expected destination_details auto-wired to saved method, got: %q", withDest)
	}
	if withType != "instapay" {
		t.Errorf("expected payout_method_type 'instapay', got: %q", withType)
	}
	if withStat != "pending" {
		t.Errorf("expected withdrawal status 'pending', got: %q", withStat)
	}
}
