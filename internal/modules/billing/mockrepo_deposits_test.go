package billing

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func (m *mockBillingRepo) UpdatePaymentMethod(_ context.Context, pm *UserPaymentMethod) error {
	if existing, ok := m.methods[pm.ID]; ok && existing.UserID == pm.UserID {
		m.methods[pm.ID] = pm
		return nil
	}
	return apperr.NotFound("payment_method")
}

func (m *mockBillingRepo) SetDefaultPaymentMethod(_ context.Context, userID, id int64) error {
	for _, pm := range m.methods {
		if pm.UserID == userID {
			pm.IsDefault = (pm.ID == id)
		}
	}
	return nil
}

func (m *mockBillingRepo) DeletePaymentMethod(_ context.Context, _, id int64) error {
	delete(m.methods, id)
	return nil
}

func (m *mockBillingRepo) ListPlatformPaymentMethods(_ context.Context, onlyActive bool) ([]*PlatformPaymentMethod, error) {
	return nil, nil
}

func (m *mockBillingRepo) GetPlatformPaymentMethod(_ context.Context, id string) (*PlatformPaymentMethod, error) {
	return nil, nil
}

func (m *mockBillingRepo) SavePlatformPaymentMethod(_ context.Context, pm *PlatformPaymentMethod) error {
	return nil
}

func (m *mockBillingRepo) TogglePlatformPaymentMethod(_ context.Context, id string, active bool) error {
	return nil
}

func (m *mockBillingRepo) TogglePlatformPaymentMethodCheckout(_ context.Context, id string, enabled bool) error {
	return nil
}

func (m *mockBillingRepo) DeletePlatformPaymentMethod(_ context.Context, id string) error {
	return nil
}

func (m *mockBillingRepo) AdminAdjustWallet(_ context.Context, walletID int64, amount money.Amount, reason string, actorID int64) error {
	return nil
}

func (m *mockBillingRepo) AdminListPayments(_ context.Context, limit, offset int) ([]*Payment, error) {
	return nil, nil
}

func (m *mockBillingRepo) AdminListSubscriptions(_ context.Context, limit, offset int) ([]*Subscription, error) {
	return nil, nil
}

func (m *mockBillingRepo) AdminListSubscriptionsWithTotal(_ context.Context, limit, offset int) ([]*Subscription, int, error) {
	return nil, 0, nil
}

func (m *mockBillingRepo) ListPaymentsByOrg(_ context.Context, orgID int64, limit, offset int) ([]*Payment, error) {
	return nil, nil
}

func (m *mockBillingRepo) ListPaymentsByOrgWithTotal(_ context.Context, orgID int64, limit, offset int) ([]*Payment, int, error) {
	return nil, 0, nil
}

func (m *mockBillingRepo) AdminListInvoices(_ context.Context, limit, offset int) ([]*Invoice, error) {
	return nil, nil
}

func (m *mockBillingRepo) AdminListWallets(_ context.Context, limit, offset int) ([]*Wallet, error) {
	return nil, nil
}

func (m *mockBillingRepo) EnsureAllOrgWallets(_ context.Context) error {
	return nil
}

func (m *mockBillingRepo) AdminListDetailedWallets(_ context.Context, _ WalletFilter) ([]*AdminWalletView, int, error) {
	return nil, 0, nil
}

func (m *mockBillingRepo) AdminListDetailedTransactions(_ context.Context, _ TransactionFilter) ([]*AdminWalletTransactionView, int, error) {
	return nil, 0, nil
}

func (m *mockBillingRepo) AdminListDetailedInvoices(_ context.Context, _ InvoiceFilter) ([]*AdminInvoiceView, int, error) {
	return nil, 0, nil
}

func (m *mockBillingRepo) AdminListDetailedPayments(_ context.Context, _ PaymentFilter) ([]*AdminPaymentView, int, error) {
	return nil, 0, nil
}

func (m *mockBillingRepo) ListVendorCustomerOrgs(_ context.Context, _ int64) ([]*CustomerOrgSummary, error) {
	return nil, nil
}

func (m *mockBillingRepo) AdminPerformWalletAdjustment(_ context.Context, _ int64, _ money.Amount, _ TransactionType, _ string, _ int64) error {
	return nil
}

func (m *mockBillingRepo) CreateDepositRequest(_ context.Context, _ *WalletDeposit) error {
	return nil
}

func (m *mockBillingRepo) GetDepositRequestByID(_ context.Context, _ int64) (*WalletDeposit, error) {
	return nil, nil
}

func (m *mockBillingRepo) UpdatePendingDepositRequest(_ context.Context, _ *WalletDeposit) error {
	return nil
}

func (m *mockBillingRepo) ListDepositRequestsByUser(_ context.Context, _ int64, _, _ int) ([]*WalletDeposit, error) {
	return nil, nil
}

func (m *mockBillingRepo) ListDepositRequestsByUserWithStatus(_ context.Context, _ int64, _ string, _, _ int) ([]*WalletDeposit, error) {
	return nil, nil
}

func (m *mockBillingRepo) AdminListDetailedDeposits(_ context.Context, _ DepositFilter) ([]*AdminWalletDepositView, int, error) {
	return nil, 0, nil
}

func (m *mockBillingRepo) AdminApproveDepositRequest(_ context.Context, _ int64, _ int64) (*WalletDeposit, *WalletTransaction, error) {
	return nil, nil, nil
}

func (m *mockBillingRepo) AdminRejectDepositRequest(_ context.Context, _ int64, _ int64, _ string) (*WalletDeposit, error) {
	return nil, nil
}

func (m *mockBillingRepo) AdminRefundTransaction(_ context.Context, _ int64, _ string, _ int64) (*WalletTransaction, error) {
	return nil, nil
}

func (m *mockBillingRepo) AdminRefundDeposit(_ context.Context, _ int64, _ string, _ int64) (*WalletDeposit, *WalletTransaction, error) {
	return nil, nil, nil
}

func (m *mockBillingRepo) CreateWithdrawalRequest(_ context.Context, _ *WalletWithdrawal) error {
	return nil
}

func (m *mockBillingRepo) GetWithdrawalRequestByID(_ context.Context, _ int64) (*WalletWithdrawal, error) {
	return nil, nil
}

func (m *mockBillingRepo) ListWithdrawalRequestsByUserWithStatus(_ context.Context, _ int64, _ string, _, _ int) ([]*WalletWithdrawal, error) {
	return nil, nil
}

func (m *mockBillingRepo) AdminListDetailedWithdrawals(_ context.Context, _ WithdrawalFilter) ([]*AdminWalletWithdrawalView, int, error) {
	return nil, 0, nil
}

func (m *mockBillingRepo) AdminApproveWithdrawalRequest(_ context.Context, _ int64, _ int64, _ string) (*WalletWithdrawal, *WalletTransaction, error) {
	return nil, nil, nil
}

func (m *mockBillingRepo) AdminRejectWithdrawalRequest(_ context.Context, _ int64, _ int64, _ string) (*WalletWithdrawal, error) {
	return nil, nil
}

func (m *mockBillingRepo) GetVendorPaymentStats(_ context.Context, _ int64) (*VendorPaymentStats, error) {
	return &VendorPaymentStats{}, nil
}

func (m *mockBillingRepo) RecordInvoicePayment(_ context.Context, req RecordInvoicePaymentRequest) (*Payment, error) {
	return &Payment{ID: 1, Amount: req.Amount, Method: req.Method, Status: "completed"}, nil
}

