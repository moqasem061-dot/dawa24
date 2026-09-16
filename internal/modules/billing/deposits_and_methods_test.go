package billing

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestWalletDepositAndWithdraw(t *testing.T) {
	ctx := context.Background()
	repo := newMockBillingRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// 1. Initial Deposit 500.00 EGP
	tx1, err := svc.Deposit(ctx, 42, "EGP", money.MustParse("500.00"), "test", nil, "Initial balance")
	if err != nil {
		t.Fatalf("Deposit failed: %v", err)
	}
	if tx1.BalanceAfter != money.MustParse("500.00") {
		t.Errorf("BalanceAfter = %v; want 500.00", tx1.BalanceAfter)
	}

	// 2. Withdraw 200.00 EGP
	tx2, err := svc.Withdraw(ctx, 42, "EGP", money.MustParse("200.00"), "test", nil, "Payment")
	if err != nil {
		t.Fatalf("Withdraw failed: %v", err)
	}
	if tx2.BalanceAfter != money.MustParse("300.00") {
		t.Errorf("BalanceAfter = %v; want 300.00", tx2.BalanceAfter)
	}

	// 3. Attempt to withdraw 400.00 EGP (exceeds balance 300.00 EGP) -> Should Fail
	_, err = svc.Withdraw(ctx, 42, "EGP", money.MustParse("400.00"), "test", nil, "Overdraft")
	if err == nil || apperr.KindOf(err) != apperr.KindValidation {
		t.Errorf("expected validation error on overdraft, got: %v", err)
	}

	// 4. Test Invoice Creation
	inv, err := svc.CreateInvoice(ctx, &Invoice{
		OrganizationID: 8801,
		Lines: []InvoiceLine{
			{Description: "Panadol Extra 500mg (100 boxes)", Quantity: 100, UnitPrice: money.MustParse("50.00"), TotalPrice: money.MustParse("5000.00")},
		},
		TaxAmount: money.MustParse("700.00"),
	})
	if err != nil {
		t.Fatalf("CreateInvoice failed: %v", err)
	}
	if inv.TotalAmount != money.MustParse("5700.00") {
		t.Errorf("TotalAmount = %v; want 5700.00", inv.TotalAmount)
	}

	gotInv, err := svc.GetInvoice(ctx, inv.ID)
	if err != nil {
		t.Fatalf("GetInvoice failed: %v", err)
	}
	if gotInv.ID != inv.ID {
		t.Errorf("got invoice id %d, want %d", gotInv.ID, inv.ID)
	}

	if err := svc.MarkInvoicePaid(ctx, inv.ID); err != nil {
		t.Fatalf("MarkInvoicePaid failed: %v", err)
	}

	// 5. Test Subscription and Entitlements
	repo.plans["pro"] = &Plan{
		Slug:     "pro",
		Features: map[string]string{"max_products": "1000", "ai_matching": "true"},
	}
	sub, err := svc.Subscribe(ctx, 42, nil, "pro", "month", nil)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	if sub.UserID != 42 {
		t.Errorf("got user id %d, want 42", sub.UserID)
	}

	allowed, val, err := svc.CheckEntitlement(ctx, 42, "ai_matching")
	if err != nil || !allowed || val != "true" {
		t.Errorf("CheckEntitlement failed: allowed=%v, val=%s, err=%v", allowed, val, err)
	}

	// 6. Payment Methods
	pm := &UserPaymentMethod{
		UserID:            42,
		Provider:          "Paymob",
		AccountIdentifier: "tok_123",
		IsDefault:         true,
	}
	if err := svc.AddPaymentMethod(ctx, pm); err != nil {
		t.Fatalf("AddPaymentMethod failed: %v", err)
	}
	methods, err := svc.ListPaymentMethods(ctx, 42)
	if err != nil || len(methods) != 1 {
		t.Fatalf("ListPaymentMethods failed: %v", err)
	}

	// 7. Admin methods
	adjMoney, _ := money.Parse("100.00")
	if err := svc.AdminAdjustWallet(ctx, 1, adjMoney, "Bonus credit", 1); err != nil {
		t.Fatalf("AdminAdjustWallet failed: %v", err)
	}
	_, _ = svc.AdminListPayments(ctx, 10, 0)
	_, _ = svc.AdminListSubscriptions(ctx, 10, 0)
}

func TestCreatePlanValidation(t *testing.T) {
	repo := newMockBillingRepo()
	svc := NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	if _, err := svc.CreatePlan(ctx, &Plan{}); err == nil {
		t.Fatal("expected error for empty slug/name")
	}

	p, err := svc.CreatePlan(ctx, &Plan{Slug: "basic", Name: i18n.Text{"ar": "أساسية"}, PriceMonth: money.MustParse("100.00")})
	if err != nil {
		t.Fatalf("CreatePlan failed: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("expected plan id to be set")
	}
}

func TestVendorPaymentStats_And_RecordInvoicePayment(t *testing.T) {
	repo := newMockBillingRepo()
	svc := NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	stats, err := svc.GetVendorPaymentStats(ctx, 42)
	if err != nil {
		t.Fatalf("GetVendorPaymentStats failed: %v", err)
	}
	if stats == nil {
		t.Fatal("expected non-nil stats")
	}

	amt, _ := money.Parse("1500.50")
	payment, err := svc.RecordInvoicePayment(ctx, RecordInvoicePaymentRequest{
		InvoiceID:       10,
		OrganizationID:  42,
		UserID:          100,
		Amount:          amt,
		Method:          "bank_transfer",
		ReferenceNumber: "REF-12345",
	})
	if err != nil {
		t.Fatalf("RecordInvoicePayment failed: %v", err)
	}
	if payment == nil || payment.ID == 0 {
		t.Fatal("expected valid payment record")
	}
	if payment.Amount.String() != "1500.50" {
		t.Fatalf("expected 1500.50, got %s", payment.Amount.String())
	}
}

func TestSubscribeWithWallet_RespectsAvailableBalance(t *testing.T) {
	ctx := context.Background()
	repo := newMockBillingRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	userID := int64(10)
	repo.plans["premium"] = &Plan{
		ID:         10,
		Slug:       "premium",
		Name:       i18n.Text{"ar": "باقة بريميوم"},
		PriceMonth: money.MustParse("300.00"),
		PriceYear:  money.MustParse("3000.00"),
	}

	// 1. Initial Deposit: Total Balance = 500.00 EGP
	_, err := svc.Deposit(ctx, userID, "EGP", money.MustParse("500.00"), "deposit", nil, "Top-up")
	if err != nil {
		t.Fatalf("Deposit failed: %v", err)
	}

	// 2. Add pending withdrawal of 400.00 EGP:
	// Total Balance = 500.00 EGP, Pending = 400.00 EGP, Available = 100.00 EGP
	wallet, _ := repo.GetOrCreateWallet(ctx, userID, "EGP")
	wallet.PendingWithdrawal = money.MustParse("400.00")
	wallet.AvailableBalance = money.MustParse("100.00")

	// 3. Attempt to subscribe to plan (Cost 300.00 EGP).
	// Total Balance (500) > Cost (300), BUT Available (100) < Cost (300) -> Must fail with insufficient funds!
	_, err = svc.SubscribeWithWallet(ctx, userID, nil, "premium", "monthly", false)
	if err == nil {
		t.Fatalf("expected error when available balance is insufficient for subscription, got nil")
	}
	if apperr.KindOf(err) != apperr.KindConflict {
		t.Fatalf("expected Conflict error (wallet.insufficient_funds), got: %v", err)
	}

	// 4. Reduce pending withdrawal so available balance becomes 300.00 EGP
	wallet.PendingWithdrawal = money.MustParse("200.00")
	wallet.AvailableBalance = money.MustParse("300.00")

	// 5. Subscription now succeeds
	sub, err := svc.SubscribeWithWallet(ctx, userID, nil, "premium", "monthly", false)
	if err != nil {
		t.Fatalf("expected subscription to succeed when available balance is sufficient, got: %v", err)
	}
	if sub == nil || sub.PlanID != 10 {
		t.Fatalf("expected valid subscription, got: %v", sub)
	}
}

func TestSubscribeWithWallet_UpgradeAndDescriptions(t *testing.T) {
	ctx := context.Background()
	repo := newMockBillingRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	userID := int64(20)
	orgID := int64(50)
	repo.plans["basic"] = &Plan{
		ID:         1,
		Slug:       "basic",
		Name:       i18n.Text{"ar": "باقة أساسية"},
		PriceMonth: money.Zero,
		PriceYear:  money.Zero,
	}
	repo.plans["pro"] = &Plan{
		ID:         2,
		Slug:       "pro",
		Name:       i18n.Text{"ar": "باقة المحترفين"},
		PriceMonth: money.MustParse("200.00"),
		PriceYear:  money.MustParse("2000.00"),
	}

	// 1. Initial Deposit: Total Balance = 1000.00 EGP
	_, err := svc.Deposit(ctx, userID, "EGP", money.MustParse("1000.00"), "deposit", nil, "Top-up")
	if err != nil {
		t.Fatalf("Deposit failed: %v", err)
	}

	// 2. First-time subscription to basic
	sub1, err := svc.SubscribeWithWallet(ctx, userID, &orgID, "basic", "monthly", true)
	if err != nil {
		t.Fatalf("initial subscribe failed: %v", err)
	}
	if sub1.PlanID != 1 {
		t.Fatalf("expected plan 1, got %d", sub1.PlanID)
	}

	// 3. Upgrade from basic to pro
	sub2, err := svc.SubscribeWithWallet(ctx, userID, &orgID, "pro", "monthly", true)
	if err != nil {
		t.Fatalf("upgrade subscribe failed: %v", err)
	}
	if sub2.PlanID != 2 {
		t.Fatalf("expected plan 2, got %d", sub2.PlanID)
	}

	// Check that wallet transaction recorded upgrade details
	wallet, _ := repo.GetOrCreateWallet(ctx, userID, "EGP")
	txs := repo.transactions[wallet.ID]
	if len(txs) < 2 {
		t.Fatalf("expected at least 2 transactions, got %d", len(txs))
	}
	upgradeTx := txs[len(txs)-1]
	if upgradeTx.ReferenceType != "subscription_upgrade" {
		t.Errorf("expected reference_type 'subscription_upgrade', got %q", upgradeTx.ReferenceType)
	}
	if upgradeTx.ReferenceID == nil || *upgradeTx.ReferenceID != sub2.ID {
		t.Errorf("expected reference_id %d, got %v", sub2.ID, upgradeTx.ReferenceID)
	}
	if !strings.Contains(upgradeTx.Description, "تصفير الاستهلاك القديم") {
		t.Errorf("expected description to mention resetting consumption, got %q", upgradeTx.Description)
	}
}
