package integration_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	billingPostgres "github.com/moqasem061-dot/dawa24/internal/modules/billing/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/identity"
	identityPostgres "github.com/moqasem061-dot/dawa24/internal/modules/identity/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/ui"
)

// S22 on the real database. A paid session plan is charged to the
// payer's wallet and raises the buyer's sign-in limit in one transaction. The
// requests go through the settings handler, the way the browser reaches it.

// payPlanWorld is one purchase handler wired the way the composition root wires it:
// the identity service receives its wallet debit from the billing repository.
type payPlanWorld struct {
	db       *database.DB
	billRepo *billingPostgres.Repository
	handler  *ui.UIHandler
	suffix   string
}

func newPayPlanWorld(t *testing.T) *payPlanWorld {
	t.Helper()
	db := wrOpenDB(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	billRepo := billingPostgres.NewRepository(db)
	idSvc := identity.NewService(identityPostgres.NewRepository(db), nil, logger)
	idSvc.SetSessionPlanDebiter(func(ctx context.Context, tx pgx.Tx, c identity.SessionPlanCharge) (bool, error) {
		return billRepo.DebitSessionPlanInTx(ctx, tx, c.PayerUserID, c.BuyerUserID, c.PlanID, c.Amount, c.Key, c.Description)
	})
	n := wrSeq.Add(1)
	return &payPlanWorld{
		db:       db,
		billRepo: billRepo,
		handler:  ui.NewUIHandler(nil, nil, nil, nil, nil, idSvc, nil, nil, nil, nil, nil, nil, nil, nil, logger),
		suffix:   fmt.Sprintf("%d-%d", time.Now().UnixNano()%1000000000, n),
	}
}

// newBuyer creates an active user with a wallet holding the deposit (a decimal
// string such as "300.00") and returns the user id and the wallet id.
func (w *payPlanWorld) newBuyer(t *testing.T, deposit string) (int64, int64) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	n := wrSeq.Add(1)
	var userID int64
	if err := w.db.Pool().QueryRow(ctx, `INSERT INTO identity.users (name, email, password_hash, role, status)
		VALUES ($1, $2, '$2a$10$abcdefghijklmnopqrstuu', 'user', 'active') RETURNING id`,
		`{"ar":"مستخدم `+w.suffix+`"}`, fmt.Sprintf("sp-%s-%d@dawa24.test", w.suffix, n)).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	wallet, err := w.billRepo.GetOrCreateWallet(ctx, userID, "EGP")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.billRepo.RecordTransaction(ctx, wallet.ID, billing.TxDeposit, money.MustParse(deposit), "seed", nil, "seed deposit"); err != nil {
		t.Fatal(err)
	}
	return userID, wallet.ID
}

// newPlan inserts a session plan with the given price, free flag and state.
func (w *payPlanWorld) newPlan(t *testing.T, price string, isFree, active bool, maxSessions int) int64 {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	n := wrSeq.Add(1)
	var planID int64
	if err := w.db.Pool().QueryRow(ctx, `INSERT INTO identity.session_plans (name, max_login_sessions, price, duration_days, is_free, is_active)
		VALUES ($1::jsonb, $2, $3, 30, $4, $5) RETURNING id`,
		fmt.Sprintf(`{"ar":"خطة %s-%d","en":"Plan %s-%d"}`, w.suffix, n, w.suffix, n), maxSessions, price, isFree, active).Scan(&planID); err != nil {
		t.Fatal(err)
	}
	return planID
}

// post sends one purchase of planID as userID. It is safe to call from several
// goroutines at once.
func (w *payPlanWorld) post(userID, planID int64, form url.Values) *httptest.ResponseRecorder {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(planID, 10))
	ctx := context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
	ctx = authctx.WithActor(ctx, authctx.Actor{UserID: userID})
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/settings/security/plan/"+strconv.FormatInt(planID, 10), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	w.handler.SettingsSessionPlanPurchaseSubmit(rec, req)
	return rec
}

// race sends one request per form at the same moment, released by one barrier.
func (w *payPlanWorld) race(userID, planID int64, forms []url.Values) []*httptest.ResponseRecorder {
	recs := make([]*httptest.ResponseRecorder, len(forms))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range forms {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			recs[i] = w.post(userID, planID, forms[i])
		}(i)
	}
	close(start)
	wg.Wait()
	return recs
}

// payNotice returns the notice kind ("success" or "error") and the message that
// the redirect carries.
func payNotice(rec *httptest.ResponseRecorder) (string, string) {
	u, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		return "", ""
	}
	q := u.Query()
	kind := q.Get("notice")
	if kind == "" {
		kind = q.Get("notice_type")
	}
	msg := q.Get("msg")
	if msg == "" {
		msg = q.Get("notice_msg")
	}
	return kind, msg
}

func (w *payPlanWorld) balance(t *testing.T, walletID int64) string {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	var s string
	if err := w.db.Pool().QueryRow(ctx, `SELECT balance_after::text FROM billing.wallet_transactions WHERE wallet_id = $1 ORDER BY id DESC LIMIT 1;`, walletID).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (w *payPlanWorld) ledgerCount(t *testing.T, walletID int64) int {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	var n int
	if err := w.db.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM billing.wallet_transactions WHERE wallet_id = $1;`, walletID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// purchaseCount counts the ledger rows that a session-plan purchase with this
// token wrote. The token is the last part of the ledger key.
func (w *payPlanWorld) purchaseCount(t *testing.T, walletID int64, token string) int {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	var n int
	if err := w.db.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM billing.wallet_transactions
		WHERE wallet_id = $1 AND idempotency_key LIKE 'session-plan:%' AND idempotency_key LIKE '%:' || $2;`, walletID, token).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// purchaseAmount returns the amount of the purchase ledger row for the token, or
// "none" when there is no such row.
func (w *payPlanWorld) purchaseAmount(t *testing.T, walletID int64, token string) string {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	var s string
	if err := w.db.Pool().QueryRow(ctx, `SELECT COALESCE((SELECT amount::text FROM billing.wallet_transactions
		WHERE wallet_id = $1 AND idempotency_key LIKE 'session-plan:%' AND idempotency_key LIKE '%:' || $2
		ORDER BY id LIMIT 1), 'none');`, walletID, token).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// debitCount counts the ledger rows with exactly this amount, for example "-250.50".
func (w *payPlanWorld) debitCount(t *testing.T, walletID int64, amount string) int {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	var n int
	if err := w.db.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM billing.wallet_transactions WHERE wallet_id = $1 AND amount = $2::numeric;`, walletID, amount).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// negativeBalances counts ledger rows whose running balance went below zero.
func (w *payPlanWorld) negativeBalances(t *testing.T, walletID int64) int {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	var n int
	if err := w.db.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM billing.wallet_transactions WHERE wallet_id = $1 AND balance_after < 0;`, walletID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// maxSessions returns the buyer's sign-in limit, or -1 when no security row exists.
func (w *payPlanWorld) maxSessions(t *testing.T, userID int64) int {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	var n int
	if err := w.db.Pool().QueryRow(ctx, `SELECT COALESCE((SELECT max_login_sessions FROM identity.user_security WHERE user_id = $1), -1);`, userID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSessionPlanPaymentPaidPlanDebitsExactPriceAndRaisesLimit(t *testing.T) {
	w := newPayPlanWorld(t)
	userID, walletID := w.newBuyer(t, "1000.00")
	planID := w.newPlan(t, "250.50", false, true, 7)
	token := "tok-paid-" + w.suffix

	rec := w.post(userID, planID, url.Values{"purchase_token": {token}})
	if kind, msg := payNotice(rec); kind != "success" {
		t.Fatalf("notice = %q (%s), want success", kind, msg)
	}
	if got := w.purchaseAmount(t, walletID, token); got != "-250.50" {
		t.Fatalf("purchase ledger amount = %s, want -250.50", got)
	}
	if got := w.purchaseCount(t, walletID, token); got != 1 {
		t.Fatalf("purchase ledger rows = %d, want 1", got)
	}
	if got := w.balance(t, walletID); got != "749.50" {
		t.Fatalf("balance = %s, want 749.50", got)
	}
	if got := w.maxSessions(t, userID); got != 7 {
		t.Fatalf("max login sessions = %d, want 7", got)
	}
}

func TestSessionPlanPaymentShortBalanceWritesNothing(t *testing.T) {
	w := newPayPlanWorld(t)
	userID, walletID := w.newBuyer(t, "100.00")
	planID := w.newPlan(t, "250.50", false, true, 7)
	rowsBefore := w.ledgerCount(t, walletID)

	rec := w.post(userID, planID, url.Values{"purchase_token": {"tok-short-" + w.suffix}})
	kind, msg := payNotice(rec)
	if kind != "error" {
		t.Fatalf("notice = %q, want error", kind)
	}
	if !strings.Contains(msg, "رصيد") || !strings.Contains(msg, "balance") {
		t.Fatalf("message %q is not bilingual about the wallet balance", msg)
	}
	if got := w.balance(t, walletID); got != "100.00" {
		t.Fatalf("balance = %s, want 100.00 (unchanged)", got)
	}
	if got := w.ledgerCount(t, walletID); got != rowsBefore {
		t.Fatalf("ledger rows = %d, want %d (no row written)", got, rowsBefore)
	}
	if got := w.maxSessions(t, userID); got != -1 {
		t.Fatalf("max login sessions = %d, want unset (the limit must not be raised)", got)
	}
}

func TestSessionPlanPaymentFreePlanMakesNoDebit(t *testing.T) {
	w := newPayPlanWorld(t)
	for _, tc := range []struct {
		name   string
		price  string
		isFree bool
	}{
		{name: "flagged free", price: "0.00", isFree: true},
		{name: "zero price", price: "0.00", isFree: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			userID, walletID := w.newBuyer(t, "50.00")
			planID := w.newPlan(t, tc.price, tc.isFree, true, 4)
			rowsBefore := w.ledgerCount(t, walletID)

			rec := w.post(userID, planID, url.Values{"purchase_token": {"tok-free-" + w.suffix + tc.name[:2]}})
			if kind, msg := payNotice(rec); kind != "success" {
				t.Fatalf("notice = %q (%s), want success", kind, msg)
			}
			if got := w.ledgerCount(t, walletID); got != rowsBefore {
				t.Fatalf("ledger rows = %d, want %d (a free plan has no debit row)", got, rowsBefore)
			}
			if got := w.balance(t, walletID); got != "50.00" {
				t.Fatalf("balance = %s, want 50.00", got)
			}
			if got := w.maxSessions(t, userID); got != 4 {
				t.Fatalf("max login sessions = %d, want 4", got)
			}
		})
	}
}

func TestSessionPlanPaymentInactivePlanIsRefused(t *testing.T) {
	w := newPayPlanWorld(t)
	userID, walletID := w.newBuyer(t, "1000.00")
	planID := w.newPlan(t, "250.50", false, false, 7)
	rowsBefore := w.ledgerCount(t, walletID)

	rec := w.post(userID, planID, url.Values{"purchase_token": {"tok-inactive-" + w.suffix}})
	if kind, _ := payNotice(rec); kind != "error" {
		t.Fatalf("notice = %q, want error", kind)
	}
	if got := w.ledgerCount(t, walletID); got != rowsBefore {
		t.Fatalf("ledger rows = %d, want %d", got, rowsBefore)
	}
	if got := w.balance(t, walletID); got != "1000.00" {
		t.Fatalf("balance = %s, want 1000.00", got)
	}
	if got := w.maxSessions(t, userID); got != -1 {
		t.Fatalf("max login sessions = %d, want unset", got)
	}
}

// The price, the free flag and the limit come from the stored plan. Values the
// client sends in the form are ignored.
func TestSessionPlanPaymentClientSuppliedPriceIsIgnored(t *testing.T) {
	w := newPayPlanWorld(t)
	userID, walletID := w.newBuyer(t, "1000.00")
	planID := w.newPlan(t, "250.50", false, true, 9)
	token := "tok-client-" + w.suffix

	rec := w.post(userID, planID, url.Values{
		"purchase_token":     {token},
		"price":              {"0.01"},
		"amount":             {"0.01"},
		"plan_price":         {"0.00"},
		"is_free":            {"true"},
		"max_login_sessions": {"500"},
	})
	if kind, msg := payNotice(rec); kind != "success" {
		t.Fatalf("notice = %q (%s), want success", kind, msg)
	}
	if got := w.purchaseAmount(t, walletID, token); got != "-250.50" {
		t.Fatalf("purchase ledger amount = %s, want the stored price -250.50", got)
	}
	if got := w.balance(t, walletID); got != "749.50" {
		t.Fatalf("balance = %s, want 749.50", got)
	}
	if got := w.maxSessions(t, userID); got != 9 {
		t.Fatalf("max login sessions = %d, want the stored 9", got)
	}
}
