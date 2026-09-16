package billing

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Repository defines the storage contract for wallet transactions, payments, plans, and entitlements.
type Repository interface {
	GetOrCreateWallet(ctx context.Context, userID int64, currency string) (*Wallet, error)
	GetWallet(ctx context.Context, id int64) (*Wallet, error)
	RecordTransaction(ctx context.Context, walletID int64, txType TransactionType, delta money.Amount, refType string, refID *int64, desc string) (*WalletTransaction, error)
	ListTransactions(ctx context.Context, walletID int64, limit, offset int) ([]*WalletTransaction, error)
	ListTransactionsWithTotal(ctx context.Context, walletID int64, limit, offset int) ([]*WalletTransaction, int, error)
	ListTransactionsWithTypeTotal(ctx context.Context, walletID int64, txType string, limit, offset int) ([]*WalletTransaction, int, error)

	CreatePayment(ctx context.Context, p *Payment) error
	GetPaymentByID(ctx context.Context, id int64) (*Payment, error)

	ListPlans(ctx context.Context) ([]*Plan, error)
	AdminListPlans(ctx context.Context) ([]*Plan, error)
	GetPlanByID(ctx context.Context, id int64) (*Plan, error)
	GetPlanBySlug(ctx context.Context, slug string) (*Plan, error)
	GetDefaultPlan(ctx context.Context) (*Plan, error)
	CreatePlan(ctx context.Context, p *Plan) error
	UpdatePlan(ctx context.Context, p *Plan) error
	TogglePlanActive(ctx context.Context, id int64) error
	SetDefaultPlan(ctx context.Context, id int64) error
	DeletePlan(ctx context.Context, id int64) error
	CreateSubscription(ctx context.Context, sub *Subscription) error
	GetActiveSubscription(ctx context.Context, userID int64) (*Subscription, error)
	GetActiveSubscriptionByOrg(ctx context.Context, orgID int64) (*Subscription, error)
	ListDueSubscriptionsForRenewal(ctx context.Context, now time.Time) ([]*Subscription, error)
	UpdateSubscriptionStatus(ctx context.Context, id int64, status SubscriptionStatus, renewalAttempts int) error
	RenewSubscription(ctx context.Context, subID int64, walletID int64, cost money.Amount, newExpiresAt time.Time, details string) error
	CheckEntitlement(ctx context.Context, userID int64, featureKey string) (bool, string, error)
	CheckOrgEntitlement(ctx context.Context, orgID, userID int64, featureKey string) (bool, error)
	GetSetting(ctx context.Context, key string) (string, error)

	CreateInvoice(ctx context.Context, inv *Invoice) error
	GetInvoiceByID(ctx context.Context, id int64) (*Invoice, error)
	GetInvoiceByOrderID(ctx context.Context, orderID int64) (*Invoice, error)
	UpdateInvoiceStatus(ctx context.Context, id int64, status InvoiceStatus) error
	ListInvoicesByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*Invoice, error)
	ListInvoicesByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*Invoice, int, error)

	AddPaymentMethod(ctx context.Context, pm *UserPaymentMethod) error
	GetPaymentMethodByID(ctx context.Context, userID, id int64) (*UserPaymentMethod, error)
	ListPaymentMethods(ctx context.Context, userID int64) ([]*UserPaymentMethod, error)
	UpdatePaymentMethod(ctx context.Context, pm *UserPaymentMethod) error
	SetDefaultPaymentMethod(ctx context.Context, userID, id int64) error
	DeletePaymentMethod(ctx context.Context, userID, id int64) error

	ListPlatformPaymentMethods(ctx context.Context, onlyActive bool) ([]*PlatformPaymentMethod, error)
	GetPlatformPaymentMethod(ctx context.Context, id string) (*PlatformPaymentMethod, error)
	SavePlatformPaymentMethod(ctx context.Context, pm *PlatformPaymentMethod) error
	TogglePlatformPaymentMethod(ctx context.Context, id string, active bool) error
	TogglePlatformPaymentMethodCheckout(ctx context.Context, id string, enabled bool) error
	DeletePlatformPaymentMethod(ctx context.Context, id string) error

	CreateDepositRequest(ctx context.Context, dep *WalletDeposit) error
	GetDepositRequestByID(ctx context.Context, id int64) (*WalletDeposit, error)
	UpdatePendingDepositRequest(ctx context.Context, dep *WalletDeposit) error
	ListDepositRequestsByUser(ctx context.Context, userID int64, limit, offset int) ([]*WalletDeposit, error)
	ListDepositRequestsByUserWithStatus(ctx context.Context, userID int64, status string, limit, offset int) ([]*WalletDeposit, error)
	AdminListDetailedDeposits(ctx context.Context, filter DepositFilter) ([]*AdminWalletDepositView, int, error)
	AdminApproveDepositRequest(ctx context.Context, depositID int64, reviewerID int64) (*WalletDeposit, *WalletTransaction, error)
	AdminRejectDepositRequest(ctx context.Context, depositID int64, reviewerID int64, reason string) (*WalletDeposit, error)

	CreateWithdrawalRequest(ctx context.Context, w *WalletWithdrawal) error
	GetWithdrawalRequestByID(ctx context.Context, id int64) (*WalletWithdrawal, error)
	ListWithdrawalRequestsByUserWithStatus(ctx context.Context, userID int64, status string, limit, offset int) ([]*WalletWithdrawal, error)
	AdminListDetailedWithdrawals(ctx context.Context, filter WithdrawalFilter) ([]*AdminWalletWithdrawalView, int, error)
	AdminApproveWithdrawalRequest(ctx context.Context, withdrawalID int64, reviewerID int64, transferReceiptURL string) (*WalletWithdrawal, *WalletTransaction, error)
	AdminRejectWithdrawalRequest(ctx context.Context, withdrawalID int64, reviewerID int64, reason string) (*WalletWithdrawal, error)

	ListPaymentsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*Payment, error)
	ListPaymentsByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*Payment, int, error)

	AdminListSubscriptions(ctx context.Context, limit, offset int) ([]*Subscription, error)
	AdminListSubscriptionsWithTotal(ctx context.Context, limit, offset int) ([]*Subscription, int, error)
	AdminAdjustWallet(ctx context.Context, walletID int64, amount money.Amount, reason string, actorID int64) error
	AdminPerformWalletAdjustment(ctx context.Context, walletID int64, amount money.Amount, txType TransactionType, reason string, actorID int64) error
	AdminListPayments(ctx context.Context, limit, offset int) ([]*Payment, error)
	EnsureAllOrgWallets(ctx context.Context) error
	AdminListDetailedWallets(ctx context.Context, filter WalletFilter) ([]*AdminWalletView, int, error)
	AdminListDetailedTransactions(ctx context.Context, filter TransactionFilter) ([]*AdminWalletTransactionView, int, error)
	AdminListDetailedInvoices(ctx context.Context, filter InvoiceFilter) ([]*AdminInvoiceView, int, error)
	AdminListDetailedPayments(ctx context.Context, filter PaymentFilter) ([]*AdminPaymentView, int, error)
	ListVendorCustomerOrgs(ctx context.Context, vendorOrgID int64) ([]*CustomerOrgSummary, error)
	GetVendorPaymentStats(ctx context.Context, orgID int64) (*VendorPaymentStats, error)
	RecordInvoicePayment(ctx context.Context, req RecordInvoicePaymentRequest) (*Payment, error)
	AdminGetFinanceStats(ctx context.Context) (*AdminFinanceStats, error)
	AdminRefundTransaction(ctx context.Context, transactionID int64, reason string, actorID int64) (*WalletTransaction, error)
	AdminRefundDeposit(ctx context.Context, depositID int64, reason string, actorID int64) (*WalletDeposit, *WalletTransaction, error)
}
