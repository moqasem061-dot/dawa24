package http_test

import (
	"context"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) GetOrCreateWallet(ctx context.Context, userID int64, currency string) (*billing.Wallet, error) {
	r.fail("GetOrCreateWallet"); return nil, nil
}
func (r stubRepo) GetWallet(ctx context.Context, id int64) (*billing.Wallet, error) {
	r.fail("GetWallet"); return nil, nil
}
func (r stubRepo) RecordTransaction(ctx context.Context, walletID int64, txType billing.TransactionType, delta money.Amount, refType string, refID *int64, desc string) (*billing.WalletTransaction, error) {
	r.fail("RecordTransaction"); return nil, nil
}
func (r stubRepo) ListTransactions(ctx context.Context, walletID int64, limit, offset int) ([]*billing.WalletTransaction, error) {
	r.fail("ListTransactions"); return nil, nil
}
func (r stubRepo) ListTransactionsWithTotal(ctx context.Context, walletID int64, limit, offset int) ([]*billing.WalletTransaction, int, error) {
	r.fail("ListTransactionsWithTotal"); return nil, 0, nil
}
func (r stubRepo) ListTransactionsWithTypeTotal(ctx context.Context, walletID int64, txType string, limit, offset int) ([]*billing.WalletTransaction, int, error) {
	r.fail("ListTransactionsWithTypeTotal"); return nil, 0, nil
}

func (r stubRepo) CreatePayment(ctx context.Context, p *billing.Payment) error {
	r.fail("CreatePayment"); return nil
}
func (r stubRepo) GetPaymentByID(ctx context.Context, id int64) (*billing.Payment, error) {
	r.fail("GetPaymentByID"); return nil, nil
}
func (r stubRepo) ListPlans(ctx context.Context) ([]*billing.Plan, error) {
	r.fail("ListPlans"); return nil, nil
}
func (r stubRepo) AdminListPlans(ctx context.Context) ([]*billing.Plan, error) {
	r.fail("AdminListPlans"); return nil, nil
}
func (r stubRepo) CreatePlan(ctx context.Context, p *billing.Plan) error {
	r.fail("CreatePlan"); return nil
}
func (r stubRepo) GetPlanByID(ctx context.Context, id int64) (*billing.Plan, error) {
	r.fail("GetPlanByID"); return nil, nil
}
func (r stubRepo) GetPlanBySlug(ctx context.Context, slug string) (*billing.Plan, error) {
	r.fail("GetPlanBySlug")
	return nil, nil
}
func (r stubRepo) GetDefaultPlan(ctx context.Context) (*billing.Plan, error) {
	r.fail("GetDefaultPlan")
	return nil, nil
}
func (r stubRepo) UpdatePlan(ctx context.Context, p *billing.Plan) error {
	r.fail("UpdatePlan")
	return nil
}
func (r stubRepo) TogglePlanActive(ctx context.Context, id int64) error {
	r.fail("TogglePlanActive")
	return nil
}
func (r stubRepo) SetDefaultPlan(ctx context.Context, id int64) error {
	r.fail("SetDefaultPlan")
	return nil
}
func (r stubRepo) DeletePlan(ctx context.Context, id int64) error {
	r.fail("DeletePlan")
	return nil
}
func (r stubRepo) CreateSubscription(ctx context.Context, sub *billing.Subscription) error {
	r.fail("CreateSubscription")
	return nil
}
func (r stubRepo) GetActiveSubscription(ctx context.Context, userID int64) (*billing.Subscription, error) {
	r.fail("GetActiveSubscription")
	return nil, nil
}
func (r stubRepo) GetActiveSubscriptionByOrg(ctx context.Context, orgID int64) (*billing.Subscription, error) {
	r.fail("GetActiveSubscriptionByOrg")
	return nil, nil
}
func (r stubRepo) ListDueSubscriptionsForRenewal(ctx context.Context, now time.Time) ([]*billing.Subscription, error) {
	r.fail("ListDueSubscriptionsForRenewal")
	return nil, nil
}
func (r stubRepo) UpdateSubscriptionStatus(ctx context.Context, id int64, status billing.SubscriptionStatus, renewalAttempts int) error {
	r.fail("UpdateSubscriptionStatus")
	return nil
}
func (r stubRepo) RenewSubscription(ctx context.Context, subID int64, walletID int64, cost money.Amount, newExpiresAt time.Time, details string) error {
	r.fail("RenewSubscription")
	return nil
}
func (r stubRepo) CheckEntitlement(ctx context.Context, userID int64, featureKey string) (bool, string, error) {
	r.fail("CheckEntitlement")
	return false, "", nil
}

func (r stubRepo) CreateInvoice(ctx context.Context, inv *billing.Invoice) error {
	r.fail("CreateInvoice")
	return nil
}
func (r stubRepo) GetInvoiceByID(ctx context.Context, id int64) (*billing.Invoice, error) {
	r.fail("GetInvoiceByID")
	return nil, nil
}
func (r stubRepo) GetInvoiceByOrderID(ctx context.Context, orderID int64) (*billing.Invoice, error) {
	r.fail("GetInvoiceByOrderID")
	return nil, nil
}
func (r stubRepo) UpdateInvoiceStatus(ctx context.Context, id int64, status billing.InvoiceStatus) error {
	r.fail("UpdateInvoiceStatus")
	return nil
}
func (r stubRepo) ListInvoicesByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Invoice, error) {
	r.fail("ListInvoicesByOrg")
	return nil, nil
}
func (r stubRepo) ListInvoicesByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Invoice, int, error) {
	r.fail("ListInvoicesByOrgWithTotal")
	return nil, 0, nil
}

func (r stubRepo) AddPaymentMethod(ctx context.Context, pm *billing.UserPaymentMethod) error {
	r.fail("AddPaymentMethod")
	return nil
}
func (r stubRepo) GetPaymentMethodByID(ctx context.Context, userID, id int64) (*billing.UserPaymentMethod, error) {
	r.fail("GetPaymentMethodByID")
	return nil, nil
}
func (r stubRepo) ListPaymentMethods(ctx context.Context, userID int64) ([]*billing.UserPaymentMethod, error) {
	r.fail("ListPaymentMethods")
	return nil, nil
}
func (r stubRepo) UpdatePaymentMethod(ctx context.Context, pm *billing.UserPaymentMethod) error {
	r.fail("UpdatePaymentMethod")
	return nil
}
func (r stubRepo) SetDefaultPaymentMethod(ctx context.Context, userID, id int64) error {
	r.fail("SetDefaultPaymentMethod")
	return nil
}
func (r stubRepo) DeletePaymentMethod(ctx context.Context, _, id int64) error {
	r.fail("DeletePaymentMethod")
	return nil
}
func (r stubRepo) ListPlatformPaymentMethods(ctx context.Context, onlyActive bool) ([]*billing.PlatformPaymentMethod, error) {
	r.fail("ListPlatformPaymentMethods")
	return nil, nil
}
func (r stubRepo) GetPlatformPaymentMethod(ctx context.Context, id string) (*billing.PlatformPaymentMethod, error) {
	r.fail("GetPlatformPaymentMethod")
	return nil, nil
}
func (r stubRepo) SavePlatformPaymentMethod(ctx context.Context, pm *billing.PlatformPaymentMethod) error {
	r.fail("SavePlatformPaymentMethod")
	return nil
}
func (r stubRepo) TogglePlatformPaymentMethod(ctx context.Context, id string, active bool) error {
	r.fail("TogglePlatformPaymentMethod")
	return nil
}
func (r stubRepo) TogglePlatformPaymentMethodCheckout(ctx context.Context, id string, enabled bool) error {
	r.fail("TogglePlatformPaymentMethodCheckout")
	return nil
}
func (r stubRepo) DeletePlatformPaymentMethod(ctx context.Context, id string) error {
	r.fail("DeletePlatformPaymentMethod")
	return nil
}

func (r stubRepo) AdminListSubscriptions(ctx context.Context, limit, offset int) ([]*billing.Subscription, error) {
	r.fail("AdminListSubscriptions")
	return nil, nil
}
func (r stubRepo) AdminListSubscriptionsWithTotal(ctx context.Context, limit, offset int) ([]*billing.Subscription, int, error) {
	r.fail("AdminListSubscriptionsWithTotal")
	return nil, 0, nil
}
func (r stubRepo) AdminAdjustWallet(ctx context.Context, walletID int64, amount money.Amount, reason string, actorID int64) error {
	r.fail("AdminAdjustWallet")
	return nil
}
func (r stubRepo) ListPaymentsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Payment, error) {
	r.fail("ListPaymentsByOrg")
	return nil, nil
}
func (r stubRepo) ListPaymentsByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Payment, int, error) {
	r.fail("ListPaymentsByOrgWithTotal")
	return nil, 0, nil
}
func (r stubRepo) AdminListInvoices(ctx context.Context, limit, offset int) ([]*billing.Invoice, error) {
	r.fail("AdminListInvoices")
	return nil, nil
}
func (r stubRepo) AdminListWallets(ctx context.Context, limit, offset int) ([]*billing.Wallet, error) {
	r.fail("AdminListWallets")
	return nil, nil
}
func (r stubRepo) AdminListPayments(ctx context.Context, limit, offset int) ([]*billing.Payment, error) {
	r.fail("AdminListPayments")
	return nil, nil
}
func (r stubRepo) EnsureAllOrgWallets(ctx context.Context) error {
	r.fail("EnsureAllOrgWallets")
	return nil
}
func (r stubRepo) AdminListDetailedWallets(ctx context.Context, filter billing.WalletFilter) ([]*billing.AdminWalletView, int, error) {
	r.fail("AdminListDetailedWallets")
	return nil, 0, nil
}
func (r stubRepo) AdminListDetailedTransactions(ctx context.Context, filter billing.TransactionFilter) ([]*billing.AdminWalletTransactionView, int, error) {
	r.fail("AdminListDetailedTransactions")
	return nil, 0, nil
}
func (r stubRepo) AdminListDetailedInvoices(ctx context.Context, filter billing.InvoiceFilter) ([]*billing.AdminInvoiceView, int, error) {
	r.fail("AdminListDetailedInvoices")
	return nil, 0, nil
}
func (r stubRepo) AdminListDetailedPayments(ctx context.Context, filter billing.PaymentFilter) ([]*billing.AdminPaymentView, int, error) {
	r.fail("AdminListDetailedPayments")
	return nil, 0, nil
}
func (r stubRepo) ListVendorCustomerOrgs(ctx context.Context, vendorOrgID int64) ([]*billing.CustomerOrgSummary, error) {
	r.fail("ListVendorCustomerOrgs")
	return nil, nil
}
func (r stubRepo) AdminPerformWalletAdjustment(ctx context.Context, walletID int64, amount money.Amount, txType billing.TransactionType, reason string, actorID int64) error {
	r.fail("AdminPerformWalletAdjustment")
	return nil
}
func (r stubRepo) CreateDepositRequest(ctx context.Context, dep *billing.WalletDeposit) error {
	r.fail("CreateDepositRequest")
	return nil
}
func (r stubRepo) GetDepositRequestByID(ctx context.Context, id int64) (*billing.WalletDeposit, error) {
	r.fail("GetDepositRequestByID")
	return nil, nil
}
func (r stubRepo) UpdatePendingDepositRequest(ctx context.Context, dep *billing.WalletDeposit) error {
	r.fail("UpdatePendingDepositRequest")
	return nil
}
func (r stubRepo) ListDepositRequestsByUser(ctx context.Context, userID int64, limit, offset int) ([]*billing.WalletDeposit, error) {
	r.fail("ListDepositRequestsByUser")
	return nil, nil
}
func (r stubRepo) ListDepositRequestsByUserWithStatus(ctx context.Context, userID int64, status string, limit, offset int) ([]*billing.WalletDeposit, error) {
	r.fail("ListDepositRequestsByUserWithStatus")
	return nil, nil
}
func (r stubRepo) AdminListDetailedDeposits(ctx context.Context, filter billing.DepositFilter) ([]*billing.AdminWalletDepositView, int, error) {
	r.fail("AdminListDetailedDeposits")
	return nil, 0, nil
}
func (r stubRepo) AdminApproveDepositRequest(ctx context.Context, depositID int64, reviewerID int64) (*billing.WalletDeposit, *billing.WalletTransaction, error) {
	r.fail("AdminApproveDepositRequest")
	return nil, nil, nil
}
func (r stubRepo) CheckOrgEntitlement(ctx context.Context, orgID, userID int64, featureKey string) (bool, error) {
	r.fail("CheckOrgEntitlement")
	return false, nil
}
func (r stubRepo) GetSetting(ctx context.Context, key string) (string, error) {
	r.fail("GetSetting")
	return "", nil
}
func (r stubRepo) AdminRejectDepositRequest(ctx context.Context, depositID int64, reviewerID int64, reason string) (*billing.WalletDeposit, error) {
	r.fail("AdminRejectDepositRequest")
	return nil, nil
}
func (r stubRepo) CreateWithdrawalRequest(ctx context.Context, w *billing.WalletWithdrawal) error {
	r.fail("CreateWithdrawalRequest")
	return nil
}
func (r stubRepo) GetWithdrawalRequestByID(ctx context.Context, id int64) (*billing.WalletWithdrawal, error) {
	r.fail("GetWithdrawalRequestByID")
	return nil, nil
}
func (r stubRepo) ListWithdrawalRequestsByUserWithStatus(ctx context.Context, userID int64, status string, limit, offset int) ([]*billing.WalletWithdrawal, error) {
	r.fail("ListWithdrawalRequestsByUserWithStatus")
	return nil, nil
}
func (r stubRepo) AdminListDetailedWithdrawals(ctx context.Context, filter billing.WithdrawalFilter) ([]*billing.AdminWalletWithdrawalView, int, error) {
	r.fail("AdminListDetailedWithdrawals")
	return nil, 0, nil
}
func (r stubRepo) AdminApproveWithdrawalRequest(ctx context.Context, withdrawalID int64, reviewerID int64, _ string) (*billing.WalletWithdrawal, *billing.WalletTransaction, error) {
	r.fail("AdminApproveWithdrawalRequest")
	return nil, nil, nil
}
func (r stubRepo) AdminRejectWithdrawalRequest(ctx context.Context, withdrawalID int64, reviewerID int64, reason string) (*billing.WalletWithdrawal, error) {
	r.fail("AdminRejectWithdrawalRequest")
	return nil, nil
}

type happyRepo struct{}

func (happyRepo) GetOrCreateWallet(ctx context.Context, userID int64, currency string) (*billing.Wallet, error) {
	b := money.MustParse("100.00")
	return &billing.Wallet{ID: 1, UserID: userID, Currency: currency, Balance: b, AvailableBalance: b}, nil
}
func (happyRepo) GetWallet(ctx context.Context, id int64) (*billing.Wallet, error) {
	b := money.MustParse("100.00")
	return &billing.Wallet{ID: id, UserID: 1, Currency: "EGP", Balance: b, AvailableBalance: b}, nil
}
func (happyRepo) RecordTransaction(ctx context.Context, walletID int64, txType billing.TransactionType, delta money.Amount, refType string, refID *int64, desc string) (*billing.WalletTransaction, error) {
	return &billing.WalletTransaction{ID: 1, WalletID: walletID, Type: txType, Amount: delta, BalanceAfter: delta}, nil
}
func (happyRepo) ListTransactions(ctx context.Context, walletID int64, limit, offset int) ([]*billing.WalletTransaction, error) {
	return []*billing.WalletTransaction{{ID: 1, WalletID: walletID}}, nil
}
func (happyRepo) ListTransactionsWithTotal(ctx context.Context, walletID int64, limit, offset int) ([]*billing.WalletTransaction, int, error) {
	return []*billing.WalletTransaction{{ID: 1, WalletID: walletID}}, 1, nil
}
func (happyRepo) ListTransactionsWithTypeTotal(ctx context.Context, walletID int64, txType string, limit, offset int) ([]*billing.WalletTransaction, int, error) {
	return []*billing.WalletTransaction{{ID: 1, WalletID: walletID}}, 1, nil
}
func (happyRepo) CreatePayment(ctx context.Context, p *billing.Payment) error {
	p.ID = 1
	return nil
}
func (happyRepo) GetPaymentByID(ctx context.Context, id int64) (*billing.Payment, error) {
	return &billing.Payment{ID: id, UserID: 1, Amount: money.MustParse("100.00"), Method: "fawry"}, nil
}
func (happyRepo) ListPlans(ctx context.Context) ([]*billing.Plan, error) {
	return []*billing.Plan{{ID: 1, Slug: "basic", Name: i18n.Text{"en": "Basic"}}}, nil
}
func (happyRepo) AdminListPlans(ctx context.Context) ([]*billing.Plan, error) {
	return []*billing.Plan{{ID: 1, Slug: "basic", Name: i18n.Text{"en": "Basic"}}}, nil
}
func (happyRepo) GetPlanBySlug(ctx context.Context, slug string) (*billing.Plan, error) {
	return &billing.Plan{ID: 1, Slug: slug, Name: i18n.Text{"en": "Basic"}}, nil
}

func (happyRepo) CreatePlan(ctx context.Context, p *billing.Plan) error {
	p.ID = 1
	return nil
}
func (happyRepo) GetPlanByID(ctx context.Context, id int64) (*billing.Plan, error) {
	return &billing.Plan{ID: id, Slug: "basic", Name: i18n.Text{"en": "Basic"}}, nil
}
func (happyRepo) GetDefaultPlan(ctx context.Context) (*billing.Plan, error) {
	return &billing.Plan{ID: 1, Slug: "basic", Name: i18n.Text{"en": "Basic"}, IsDefault: true}, nil
}
func (happyRepo) UpdatePlan(ctx context.Context, p *billing.Plan) error {
	return nil
}
func (happyRepo) TogglePlanActive(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) SetDefaultPlan(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) DeletePlan(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) CreateSubscription(ctx context.Context, sub *billing.Subscription) error {
	sub.ID = 1
	return nil
}
func (happyRepo) GetActiveSubscription(ctx context.Context, userID int64) (*billing.Subscription, error) {
	return &billing.Subscription{ID: 1, UserID: userID, Status: billing.SubActive}, nil
}
func (happyRepo) GetActiveSubscriptionByOrg(ctx context.Context, orgID int64) (*billing.Subscription, error) {
	return &billing.Subscription{ID: 1, OrganizationID: &orgID, Status: billing.SubActive}, nil
}
func (happyRepo) ListDueSubscriptionsForRenewal(ctx context.Context, now time.Time) ([]*billing.Subscription, error) {
	return []*billing.Subscription{{ID: 1, UserID: 1, Status: billing.SubActive, AutoRenew: true}}, nil
}
func (happyRepo) UpdateSubscriptionStatus(ctx context.Context, id int64, status billing.SubscriptionStatus, renewalAttempts int) error {
	return nil
}
func (happyRepo) RenewSubscription(ctx context.Context, subID int64, walletID int64, cost money.Amount, newExpiresAt time.Time, details string) error {
	return nil
}
