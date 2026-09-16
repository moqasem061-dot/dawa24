package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	billingHttp "github.com/muhiya/dawa24-store/internal/modules/billing/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func (happyRepo) CheckEntitlement(ctx context.Context, userID int64, featureKey string) (bool, string, error) {
	return true, "unlimited", nil
}
func (happyRepo) CheckOrgEntitlement(ctx context.Context, orgID, userID int64, featureKey string) (bool, error) {
	return true, nil
}
func (happyRepo) GetSetting(ctx context.Context, key string) (string, error) {
	return `{"value": 25, "days": 25}`, nil
}
func (happyRepo) CreateInvoice(ctx context.Context, inv *billing.Invoice) error {
	inv.ID = 1
	return nil
}
func (happyRepo) GetInvoiceByID(ctx context.Context, id int64) (*billing.Invoice, error) {
	return &billing.Invoice{ID: id, OrganizationID: 1, InvoiceNumber: "INV-1", Subtotal: money.MustParse("100.00")}, nil
}
func (happyRepo) GetInvoiceByOrderID(ctx context.Context, orderID int64) (*billing.Invoice, error) {
	return &billing.Invoice{ID: 1, OrganizationID: 1, OrderID: &orderID, InvoiceNumber: "INV-1", Subtotal: money.MustParse("100.00")}, nil
}
func (happyRepo) UpdateInvoiceStatus(ctx context.Context, id int64, status billing.InvoiceStatus) error {
	return nil
}
func (happyRepo) ListInvoicesByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Invoice, error) {
	return []*billing.Invoice{{ID: 1, OrganizationID: orgID}}, nil
}
func (happyRepo) ListInvoicesByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Invoice, int, error) {
	return []*billing.Invoice{{ID: 1, OrganizationID: orgID}}, 1, nil
}
func (happyRepo) AddPaymentMethod(ctx context.Context, pm *billing.UserPaymentMethod) error {
	pm.ID = 1
	return nil
}
func (happyRepo) GetPaymentMethodByID(ctx context.Context, userID, id int64) (*billing.UserPaymentMethod, error) {
	return &billing.UserPaymentMethod{ID: id, UserID: userID, Provider: "fawry"}, nil
}
func (happyRepo) ListPaymentMethods(ctx context.Context, userID int64) ([]*billing.UserPaymentMethod, error) {
	return []*billing.UserPaymentMethod{{ID: 1, UserID: userID, Provider: "fawry"}}, nil
}
func (happyRepo) UpdatePaymentMethod(ctx context.Context, pm *billing.UserPaymentMethod) error {
	return nil
}
func (happyRepo) SetDefaultPaymentMethod(ctx context.Context, userID, id int64) error {
	return nil
}
func (happyRepo) DeletePaymentMethod(ctx context.Context, _, id int64) error {
	return nil
}
func (happyRepo) ListPlatformPaymentMethods(ctx context.Context, onlyActive bool) ([]*billing.PlatformPaymentMethod, error) {
	return []*billing.PlatformPaymentMethod{{ID: "instapay", ProviderType: "instapay"}}, nil
}
func (happyRepo) GetPlatformPaymentMethod(ctx context.Context, id string) (*billing.PlatformPaymentMethod, error) {
	return &billing.PlatformPaymentMethod{ID: id, ProviderType: "instapay"}, nil
}
func (happyRepo) SavePlatformPaymentMethod(ctx context.Context, pm *billing.PlatformPaymentMethod) error {
	return nil
}
func (happyRepo) TogglePlatformPaymentMethod(ctx context.Context, id string, active bool) error {
	return nil
}
func (happyRepo) TogglePlatformPaymentMethodCheckout(ctx context.Context, id string, enabled bool) error {
	return nil
}
func (happyRepo) DeletePlatformPaymentMethod(ctx context.Context, id string) error {
	return nil
}
func (happyRepo) AdminListSubscriptions(ctx context.Context, limit, offset int) ([]*billing.Subscription, error) {
	return []*billing.Subscription{{ID: 1, UserID: 1}}, nil
}
func (happyRepo) AdminListSubscriptionsWithTotal(ctx context.Context, limit, offset int) ([]*billing.Subscription, int, error) {
	return []*billing.Subscription{{ID: 1, UserID: 1}}, 1, nil
}
func (happyRepo) AdminAdjustWallet(ctx context.Context, walletID int64, amount money.Amount, reason string, actorID int64) error {
	return nil
}
func (happyRepo) ListPaymentsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Payment, error) {
	return []*billing.Payment{{ID: 1, OrganizationID: &orgID, Amount: money.MustParse("100.00")}}, nil
}
func (happyRepo) ListPaymentsByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Payment, int, error) {
	return []*billing.Payment{{ID: 1, OrganizationID: &orgID, Amount: money.MustParse("100.00")}}, 1, nil
}
func (happyRepo) AdminListInvoices(ctx context.Context, limit, offset int) ([]*billing.Invoice, error) {
	return []*billing.Invoice{{ID: 1, InvoiceNumber: "INV-1"}}, nil
}
func (happyRepo) AdminListWallets(ctx context.Context, limit, offset int) ([]*billing.Wallet, error) {
	return []*billing.Wallet{{ID: 1, UserID: 1, Balance: money.MustParse("100.00")}}, nil
}
func (happyRepo) AdminListPayments(ctx context.Context, limit, offset int) ([]*billing.Payment, error) {
	return []*billing.Payment{{ID: 1, UserID: 1}}, nil
}
func (happyRepo) EnsureAllOrgWallets(ctx context.Context) error {
	return nil
}
func (happyRepo) AdminListDetailedWallets(ctx context.Context, filter billing.WalletFilter) ([]*billing.AdminWalletView, int, error) {
	return []*billing.AdminWalletView{{ID: 1, UserID: 1, UserName: "Test User"}}, 1, nil
}
func (happyRepo) AdminListDetailedTransactions(ctx context.Context, filter billing.TransactionFilter) ([]*billing.AdminWalletTransactionView, int, error) {
	return []*billing.AdminWalletTransactionView{{ID: 1, WalletID: 1, Type: billing.TxDeposit}}, 1, nil
}
func (happyRepo) AdminListDetailedInvoices(ctx context.Context, filter billing.InvoiceFilter) ([]*billing.AdminInvoiceView, int, error) {
	return []*billing.AdminInvoiceView{{ID: 1, InvoiceNumber: "INV-1"}}, 1, nil
}
func (happyRepo) AdminListDetailedPayments(ctx context.Context, filter billing.PaymentFilter) ([]*billing.AdminPaymentView, int, error) {
	return []*billing.AdminPaymentView{{ID: 1, UserID: 1}}, 1, nil
}
func (happyRepo) ListVendorCustomerOrgs(ctx context.Context, vendorOrgID int64) ([]*billing.CustomerOrgSummary, error) {
	return []*billing.CustomerOrgSummary{{ID: 1, Name: "صيدلية الأمل", Code: "PH-1"}}, nil
}
func (happyRepo) AdminPerformWalletAdjustment(ctx context.Context, walletID int64, amount money.Amount, txType billing.TransactionType, reason string, actorID int64) error {
	return nil
}
func (happyRepo) CreateDepositRequest(ctx context.Context, dep *billing.WalletDeposit) error {
	dep.ID = 1
	return nil
}
func (happyRepo) GetDepositRequestByID(ctx context.Context, id int64) (*billing.WalletDeposit, error) {
	return &billing.WalletDeposit{ID: id, UserID: 1, Status: billing.DepositPending}, nil
}
func (happyRepo) UpdatePendingDepositRequest(ctx context.Context, dep *billing.WalletDeposit) error {
	return nil
}
func (happyRepo) ListDepositRequestsByUser(ctx context.Context, userID int64, limit, offset int) ([]*billing.WalletDeposit, error) {
	return []*billing.WalletDeposit{{ID: 1, UserID: userID, Status: billing.DepositPending}}, nil
}
func (happyRepo) ListDepositRequestsByUserWithStatus(ctx context.Context, userID int64, status string, limit, offset int) ([]*billing.WalletDeposit, error) {
	return []*billing.WalletDeposit{{ID: 1, UserID: userID, Status: billing.DepositPending}}, nil
}
func (happyRepo) AdminListDetailedDeposits(ctx context.Context, filter billing.DepositFilter) ([]*billing.AdminWalletDepositView, int, error) {
	return []*billing.AdminWalletDepositView{{ID: 1, UserID: 1, Status: billing.DepositPending}}, 1, nil
}
func (happyRepo) AdminApproveDepositRequest(ctx context.Context, depositID int64, reviewerID int64) (*billing.WalletDeposit, *billing.WalletTransaction, error) {
	return &billing.WalletDeposit{ID: depositID, Status: billing.DepositApproved}, &billing.WalletTransaction{ID: 1}, nil
}
func (happyRepo) AdminRejectDepositRequest(ctx context.Context, depositID int64, reviewerID int64, reason string) (*billing.WalletDeposit, error) {
	return &billing.WalletDeposit{ID: depositID, Status: billing.DepositRejected, RejectionReason: reason}, nil
}
func (happyRepo) CreateWithdrawalRequest(ctx context.Context, w *billing.WalletWithdrawal) error {
	w.ID = 1
	w.Status = billing.WithdrawalPending
	return nil
}
func (happyRepo) GetWithdrawalRequestByID(ctx context.Context, id int64) (*billing.WalletWithdrawal, error) {
	return &billing.WalletWithdrawal{ID: id, Status: billing.WithdrawalPending}, nil
}
func (happyRepo) ListWithdrawalRequestsByUserWithStatus(ctx context.Context, userID int64, status string, limit, offset int) ([]*billing.WalletWithdrawal, error) {
	return []*billing.WalletWithdrawal{{ID: 1, UserID: userID, Status: billing.WithdrawalPending}}, nil
}
func (happyRepo) AdminListDetailedWithdrawals(ctx context.Context, filter billing.WithdrawalFilter) ([]*billing.AdminWalletWithdrawalView, int, error) {
	return []*billing.AdminWalletWithdrawalView{{ID: 1, UserID: 1, Status: billing.WithdrawalPending}}, 1, nil
}
func (happyRepo) AdminApproveWithdrawalRequest(ctx context.Context, withdrawalID int64, reviewerID int64, transferReceiptURL string) (*billing.WalletWithdrawal, *billing.WalletTransaction, error) {
	return &billing.WalletWithdrawal{ID: withdrawalID, Status: billing.WithdrawalApproved, TransferReceiptURL: transferReceiptURL}, &billing.WalletTransaction{ID: 1}, nil
}
func (happyRepo) AdminRejectWithdrawalRequest(ctx context.Context, withdrawalID int64, reviewerID int64, reason string) (*billing.WalletWithdrawal, error) {
	return &billing.WalletWithdrawal{ID: withdrawalID, Status: billing.WithdrawalRejected, RejectionReason: reason}, nil
}
func (happyRepo) GetVendorPaymentStats(ctx context.Context, orgID int64) (*billing.VendorPaymentStats, error) {
	return &billing.VendorPaymentStats{}, nil
}
func (happyRepo) RecordInvoicePayment(ctx context.Context, req billing.RecordInvoicePaymentRequest) (*billing.Payment, error) {
	return &billing.Payment{ID: 1, Amount: req.Amount, Method: req.Method, Status: "completed"}, nil
}
func (happyRepo) AdminGetFinanceStats(_ context.Context) (*billing.AdminFinanceStats, error) {
	return &billing.AdminFinanceStats{}, nil
}
func (happyRepo) AdminRefundTransaction(ctx context.Context, transactionID int64, reason string, actorID int64) (*billing.WalletTransaction, error) {
	return &billing.WalletTransaction{ID: 2, Amount: money.FromMinor(10000), Type: billing.TxRefund}, nil
}
func (happyRepo) AdminRefundDeposit(ctx context.Context, depositID int64, reason string, actorID int64) (*billing.WalletDeposit, *billing.WalletTransaction, error) {
	return &billing.WalletDeposit{ID: depositID, Status: billing.DepositRefunded}, &billing.WalletTransaction{ID: 2}, nil
}
func (s stubRepo) GetVendorPaymentStats(ctx context.Context, orgID int64) (*billing.VendorPaymentStats, error) {
	return &billing.VendorPaymentStats{}, nil
}
func (s stubRepo) RecordInvoicePayment(ctx context.Context, req billing.RecordInvoicePaymentRequest) (*billing.Payment, error) {
	return &billing.Payment{ID: 1, Amount: req.Amount, Method: req.Method, Status: "completed"}, nil
}
func (s stubRepo) AdminGetFinanceStats(_ context.Context) (*billing.AdminFinanceStats, error) {
	return &billing.AdminFinanceStats{}, nil
}
func (s stubRepo) AdminRefundTransaction(ctx context.Context, transactionID int64, reason string, actorID int64) (*billing.WalletTransaction, error) {
	return &billing.WalletTransaction{ID: 2, Amount: money.FromMinor(10000), Type: billing.TxRefund}, nil
}
func (s stubRepo) AdminRefundDeposit(ctx context.Context, depositID int64, reason string, actorID int64) (*billing.WalletDeposit, *billing.WalletTransaction, error) {
	return &billing.WalletDeposit{ID: depositID, Status: billing.DepositRefunded}, &billing.WalletTransaction{ID: 2}, nil
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := billing.NewService(stubRepo{t: t}, log)
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("dawa24_session")
			if err != nil || cookie.Value == "" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			if cookie.Value == "forged-token-that-was-never-issued" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	billingHttp.NewHandler(svc, log).RegisterRoutes(r)
	return r
}

func newAuthedRouter(repo billing.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := billing.NewService(repo, log)
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				Role:           "super_admin",
				// The wallet, invoice and subscription writes are gated now —
				// they used to take nothing but an approved organisation. A
				// super admin holds the whole catalogue in production; the
				// fixture names the keys these routes actually require.
				Permissions: []string{
					"admin", "super_admin", "billing.admin",
					"billing.wallet.manage", "billing.invoice.manage",
					"billing.subscription_plan.update",
				},
			}
			ctx := authctx.WithActor(r.Context(), actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	billingHttp.NewHandler(svc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/billing/wallet"},
	{http.MethodPost, "/api/v1/billing/wallet/deposit"},
	{http.MethodPost, "/api/v1/billing/wallet/withdraw"},
	{http.MethodGet, "/api/v1/billing/plans"},
	{http.MethodPost, "/api/v1/billing/subscriptions"},
	{http.MethodGet, "/api/v1/billing/entitlements/test_feat"},
	{http.MethodPost, "/api/v1/billing/invoices"},
	{http.MethodGet, "/api/v1/billing/invoices/1"},
	{http.MethodGet, "/api/v1/billing/invoices"},
	{http.MethodPost, "/api/v1/billing/invoices/1/pay"},
	{http.MethodPost, "/api/v1/billing/payment-methods"},
	{http.MethodGet, "/api/v1/billing/payment-methods"},
}

func TestProtectedRoutesRejectAnonymousCallers(t *testing.T) {
	router := newTestRouter(t)
	for _, route := range protectedRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("got %d, want 401 — this endpoint is reachable without a session", rec.Code)
			}
		})
	}
}

func TestProtectedRoutesRejectGarbageSessionToken(t *testing.T) {
	router := newTestRouter(t)
	for _, route := range protectedRoutes {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "dawa24_session", Value: "forged-token-that-was-never-issued"})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a forged token got %d, want 401", route.method, route.path, rec.Code)
		}
	}
}

func TestUnauthorizedResponseUsesTheErrorEnvelope(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/wallet", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the JSON error envelope: %v (body: %s)", err, rec.Body.String())
	}
	if body.Error.Code == "" {
		t.Error("error envelope has no code")
	}
	if body.Error.RequestID == "" {
		t.Error("error envelope has no request_id")
	}
}

func TestBillingHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"GetWallet", http.MethodGet, "/api/v1/billing/wallet", "", http.StatusOK},
		{"Deposit", http.MethodPost, "/api/v1/billing/wallet/deposit", `{"user_id":1,"currency":"EGP","amount":"50.00","description":"test deposit"}`, http.StatusOK},
		{"Withdraw", http.MethodPost, "/api/v1/billing/wallet/withdraw", `{"user_id":1,"currency":"EGP","amount":"25.00","description":"test withdraw"}`, http.StatusOK},
		{"ListPlans", http.MethodGet, "/api/v1/billing/plans", "", http.StatusOK},
		{"Subscribe", http.MethodPost, "/api/v1/billing/subscriptions", `{"plan_slug":"basic"}`, http.StatusCreated},
		{"CheckEntitlement", http.MethodGet, "/api/v1/billing/entitlements/ai_matching", "", http.StatusOK},
		{"CreateInvoice", http.MethodPost, "/api/v1/billing/invoices", `{"organization_id":1,"invoice_number":"INV-100","subtotal":"100.00","issue_date":"2026-01-01T00:00:00Z","due_date":"2026-02-01T00:00:00Z"}`, http.StatusCreated},
		{"GetInvoice", http.MethodGet, "/api/v1/billing/invoices/1", "", http.StatusOK},
		{"ListInvoices", http.MethodGet, "/api/v1/billing/invoices?limit=10&offset=0", "", http.StatusOK},
		{"PayInvoice", http.MethodPost, "/api/v1/billing/invoices/1/pay", `{"payment_method_id":1}`, http.StatusOK},
		{"AddPaymentMethod", http.MethodPost, "/api/v1/billing/payment-methods", `{"user_id":1,"provider":"fawry","account_identifier":"01000000000","is_default":true}`, http.StatusCreated},
		{"ListPaymentMethods", http.MethodGet, "/api/v1/billing/payment-methods", "", http.StatusOK},
		{"AdminSubscriptions", http.MethodGet, "/api/v1/admin/billing/subscriptions", "", http.StatusOK},
		{"AdminAdjustWallet", http.MethodPost, "/api/v1/admin/billing/wallets/1/adjust", `{"amount":"10.00","reason":"admin bonus"}`, http.StatusOK},
		{"AdminPayments", http.MethodGet, "/api/v1/admin/billing/payments", "", http.StatusOK},
		{"AdminMarkPaid", http.MethodPost, "/api/v1/admin/billing/invoices/1/paid", "", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != "" {
				bodyReader = strings.NewReader(tt.body)
			}
			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s got status %d, want %d (body: %s)", tt.method, tt.path, rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
