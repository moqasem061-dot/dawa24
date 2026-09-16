package ui_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

func TestTenantWallet_WithdrawalAndDeposit_Workflow_E2E(t *testing.T) {
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
		VALUES ('{"ar":"صيدلية سحب الرصيد التجريبية"}', 'صيدلية سحب الرصيد التجريبية', '{"ar":"صيدلية سحب الرصيد التجريبية"}', 'TAX-WTH-201', 'CR-WTH-201', 'customer', 'approved')
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
		VALUES ('{"ar":"دكتور السحب"}', 'withdraw_test@dawa24.eg', '$2a$10$abcdefghijklmnopqrstuu', 'user', 'active')
		RETURNING id
	`).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM identity.users WHERE id = $1`, userID)
	}()

	var adminID int64
	err = db.Pool().QueryRow(ctx, `
		INSERT INTO identity.users (name, email, password_hash, role, status)
		VALUES ('{"ar":"مدير المالية"}', 'admin_finance_test@dawa24.eg', '$2a$10$abcdefghijklmnopqrstuu', 'admin', 'active')
		RETURNING id
	`).Scan(&adminID)
	if err != nil {
		t.Fatalf("failed to insert test admin: %v", err)
	}
	defer func() {
		_, _ = db.Pool().Exec(database.AsSystem(context.Background()), `DELETE FROM identity.users WHERE id = $1`, adminID)
	}()

	customerActor := authctx.Actor{
		UserID:         userID,
		OrganizationID: customerOrgID,
		OrgType:        "customer",
		Role:           "owner",
		IsOwner:        true,
		Permissions:    []string{"pharmacy.wallet.view", "pharmacy.wallet.manage"},
	}

	adminActor := authctx.Actor{
		UserID:      adminID,
		Role:        "admin",
		IsStaff:     true,
		Permissions: []string{"billing.finance.view", "billing.payment.update", "billing.wallet.manage"},
	}

	// 2. Deposit Request with platform channel & sender account
	depForm := url.Values{
		"amount":             {"1500.00"},
		"platform_method_id": {"instapay"},
		"payment_method":     {"instapay"},
		"sender_account":     {"pharmacy_sender@instapay"},
		"reference_number":   {"IPN-99887711"},
		"notes":              {"شحن رصيد تجريبي"},
	}
	depReq := httptest.NewRequest("POST", "/customer/wallet/deposit", strings.NewReader(depForm.Encode()))
	depReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	depReq = depReq.WithContext(authctx.WithActor(depReq.Context(), customerActor))
	depRec := httptest.NewRecorder()
	handler.ServeHTTP(depRec, depReq)

	if depRec.Code != http.StatusSeeOther {
		t.Fatalf("deposit submit returned %d, expected 303", depRec.Code)
	}

	// Verify deposit in DB
	var depID int64
	var depStatus, platformMethod, senderAcc string
	err = db.Pool().QueryRow(ctx, `
		SELECT id, status, platform_method_id, sender_account
		FROM billing.wallet_deposits
		WHERE user_id = $1
		ORDER BY id DESC LIMIT 1
	`, userID).Scan(&depID, &depStatus, &platformMethod, &senderAcc)
	if err != nil {
		t.Fatalf("failed to query created deposit: %v", err)
	}
	if depStatus != "pending" {
		t.Errorf("expected deposit status 'pending', got: %s", depStatus)
	}
	if platformMethod != "instapay" {
		t.Errorf("expected platform_method_id 'instapay', got: %s", platformMethod)
	}
	if senderAcc != "pharmacy_sender@instapay" {
		t.Errorf("expected sender_account 'pharmacy_sender@instapay', got: %s", senderAcc)
	}

	// 3. Admin approves deposit
	apprReq := httptest.NewRequest("POST", fmt.Sprintf("/admin/finance/deposits/%d/approve", depID), nil)
	apprReq = apprReq.WithContext(authctx.WithActor(apprReq.Context(), adminActor))
	apprRec := httptest.NewRecorder()
	handler.ServeHTTP(apprRec, apprReq)

	if apprRec.Code != http.StatusSeeOther {
		t.Fatalf("admin deposit approve returned %d, expected 303", apprRec.Code)
	}

	// 4. Test withdrawal submit when balance is 1500: request 500
	withForm := url.Values{
		"amount":              {"500.00"},
		"payout_method_type":  {"instapay"},
		"destination_details": {"01011223344 (InstaPay)"},
		"reason":              {"سحب مستحقات تجريبية"},
	}
	withReq := httptest.NewRequest("POST", "/customer/wallet/withdraw", strings.NewReader(withForm.Encode()))
	withReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	withReq = withReq.WithContext(authctx.WithActor(withReq.Context(), customerActor))
	withRec := httptest.NewRecorder()
	handler.ServeHTTP(withRec, withReq)

	if withRec.Code != http.StatusSeeOther {
		t.Fatalf("withdrawal submit returned %d, expected 303", withRec.Code)
	}

	// Verify withdrawal created in DB with status pending
	var withID int64
	var withStatus, withDest string
	err = db.Pool().QueryRow(ctx, `
		SELECT id, status, destination_details
		FROM billing.wallet_withdrawals
		WHERE user_id = $1
		ORDER BY id DESC LIMIT 1
	`, userID).Scan(&withID, &withStatus, &withDest)
	if err != nil {
		t.Fatalf("failed to query created withdrawal: %v", err)
	}
	if withStatus != "pending" {
		t.Errorf("expected withdrawal status 'pending', got: %s", withStatus)
	}
	if withDest != "01011223344 (InstaPay)" {
		t.Errorf("expected destination_details '01011223344 (InstaPay)', got: %s", withDest)
	}

	// 5. Test withdrawal when amount exceeds balance: attempt to withdraw 50,000
	excessForm := url.Values{
		"amount":              {"50000.00"},
		"payout_method_type":  {"bank"},
		"destination_details": {"CIB EG12345678"},
	}
	excessReq := httptest.NewRequest("POST", "/customer/wallet/withdraw", strings.NewReader(excessForm.Encode()))
	excessReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	excessReq = excessReq.WithContext(authctx.WithActor(excessReq.Context(), customerActor))
	excessRec := httptest.NewRecorder()
	handler.ServeHTTP(excessRec, excessReq)

	if excessRec.Code != http.StatusSeeOther {
		t.Fatalf("excess withdrawal returned %d, expected 303", excessRec.Code)
	}
	if loc := excessRec.Header().Get("Location"); !strings.Contains(loc, "notice=error") {
		t.Errorf("expected notice=error on excess withdrawal, got: %s", loc)
	}

	// 6. Admin approves withdrawal
	admApprReq := httptest.NewRequest("POST", fmt.Sprintf("/admin/finance/withdrawals/%d/approve", withID), nil)
	admApprReq = admApprReq.WithContext(authctx.WithActor(admApprReq.Context(), adminActor))
	admApprRec := httptest.NewRecorder()
	handler.ServeHTTP(admApprRec, admApprReq)

	if admApprRec.Code != http.StatusSeeOther {
		t.Fatalf("admin withdrawal approve returned %d, expected 303", admApprRec.Code)
	}

	// Verify status is approved and ledger transaction exists
	var finalStatus string
	var txID *int64
	err = db.Pool().QueryRow(ctx, `
		SELECT status, transaction_id
		FROM billing.wallet_withdrawals
		WHERE id = $1
	`, withID).Scan(&finalStatus, &txID)
	if err != nil {
		t.Fatalf("failed to query approved withdrawal: %v", err)
	}
	if finalStatus != "approved" {
		t.Errorf("expected withdrawal status 'approved', got: %s", finalStatus)
	}
	if txID == nil || *txID <= 0 {
		t.Errorf("expected non-nil transaction_id for approved withdrawal")
	}

	// 7. Test Admin Rejecting a withdrawal
	withForm2 := url.Values{
		"amount":              {"200.00"},
		"payout_method_type":  {"wallet"},
		"destination_details": {"01122334455 (Vodafone Cash)"},
		"reason":              {"سحب تجريبي للرفض"},
	}
	withReq2 := httptest.NewRequest("POST", "/customer/wallet/withdraw", strings.NewReader(withForm2.Encode()))
	withReq2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	withReq2 = withReq2.WithContext(authctx.WithActor(withReq2.Context(), customerActor))
	withRec2 := httptest.NewRecorder()
	handler.ServeHTTP(withRec2, withReq2)

	var withID2 int64
	err = db.Pool().QueryRow(ctx, `
		SELECT id
		FROM billing.wallet_withdrawals
		WHERE user_id = $1 AND amount = 200.00
		ORDER BY id DESC LIMIT 1
	`, userID).Scan(&withID2)
	if err != nil {
		t.Fatalf("failed to query second withdrawal: %v", err)
	}

	rejForm := url.Values{
		"rejection_reason": {"بيانات المحفظة غير مسجلة باسم صاحب الحساب"},
	}
	admRejReq := httptest.NewRequest("POST", fmt.Sprintf("/admin/finance/withdrawals/%d/reject", withID2), strings.NewReader(rejForm.Encode()))
	admRejReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	admRejReq = admRejReq.WithContext(authctx.WithActor(admRejReq.Context(), adminActor))
	admRejRec := httptest.NewRecorder()
	handler.ServeHTTP(admRejRec, admRejReq)

	if admRejRec.Code != http.StatusSeeOther {
		t.Fatalf("admin withdrawal reject returned %d, expected 303", admRejRec.Code)
	}

	var rejStatus, rejReason string
	err = db.Pool().QueryRow(ctx, `
		SELECT status, rejection_reason
		FROM billing.wallet_withdrawals
		WHERE id = $1
	`, withID2).Scan(&rejStatus, &rejReason)
	if err != nil {
		t.Fatalf("failed to query rejected withdrawal: %v", err)
	}
	if rejStatus != "rejected" {
		t.Errorf("expected withdrawal status 'rejected', got: %s", rejStatus)
	}
	if rejReason != "بيانات المحفظة غير مسجلة باسم صاحب الحساب" {
		t.Errorf("expected rejection reason, got: %s", rejReason)
	}
}

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
	depReq := httptest.NewRequest("POST", "/customer/wallet/deposit", strings.NewReader(depForm.Encode()))
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
	withReq := httptest.NewRequest("POST", "/customer/wallet/withdraw", strings.NewReader(withForm.Encode()))
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
