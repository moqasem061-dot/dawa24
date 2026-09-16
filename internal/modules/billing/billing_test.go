package billing

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockBillingRepo struct {
	wallets       map[int64]*Wallet
	transactions  map[int64][]*WalletTransaction
	plans         map[string]*Plan
	subscriptions map[int64]*Subscription
	invoices      map[int64]*Invoice
	methods       map[int64]*UserPaymentMethod
	settings      map[string]string
	nextID        int64
}

func newMockBillingRepo() *mockBillingRepo {
	return &mockBillingRepo{
		wallets:       map[int64]*Wallet{},
		transactions:  map[int64][]*WalletTransaction{},
		plans:         map[string]*Plan{},
		subscriptions: map[int64]*Subscription{},
		invoices:      map[int64]*Invoice{},
		methods:       map[int64]*UserPaymentMethod{},
		settings:      map[string]string{},
		nextID:        1,
	}
}

func (m *mockBillingRepo) GetOrCreateWallet(_ context.Context, userID int64, currency string) (*Wallet, error) {
	for _, w := range m.wallets {
		if w.UserID == userID && w.Currency == currency {
			return w, nil
		}
	}
	w := &Wallet{
		ID:                m.nextID,
		UserID:            userID,
		Currency:          currency,
		Balance:           money.Zero,
		PendingWithdrawal: money.Zero,
		AvailableBalance:  money.Zero,
	}
	m.nextID++
	m.wallets[w.ID] = w
	return w, nil
}

func (m *mockBillingRepo) GetWallet(_ context.Context, id int64) (*Wallet, error) {
	w, ok := m.wallets[id]
	if !ok {
		return nil, apperr.NotFound("wallet")
	}
	return w, nil
}

func (m *mockBillingRepo) RecordTransaction(
	_ context.Context,
	walletID int64,
	txType TransactionType,
	delta money.Amount,
	refType string,
	refID *int64,
	desc string,
) (*WalletTransaction, error) {
	w, ok := m.wallets[walletID]
	if !ok {
		return nil, apperr.NotFound("wallet")
	}

	newBal, err := w.Balance.Add(delta)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if newBal.IsNegative() {
		return nil, apperr.Validation("wallet.insufficient_funds", "Insufficient funds", nil)
	}
	w.Balance = newBal
	availMinor := w.Balance.Minor() - w.PendingWithdrawal.Minor()
	if availMinor < 0 {
		availMinor = 0
	}
	w.AvailableBalance = money.FromMinor(availMinor)

	tx := &WalletTransaction{
		ID:            m.nextID,
		WalletID:      walletID,
		Type:          txType,
		Amount:        delta,
		BalanceAfter:  newBal,
		ReferenceType: refType,
		ReferenceID:   refID,
		Description:   desc,
		CreatedAt:     time.Now(),
	}
	m.nextID++
	m.transactions[walletID] = append(m.transactions[walletID], tx)
	return tx, nil
}

func (m *mockBillingRepo) ListTransactions(_ context.Context, walletID int64, limit, offset int) ([]*WalletTransaction, error) {
	return m.transactions[walletID], nil
}

func (m *mockBillingRepo) ListTransactionsWithTotal(ctx context.Context, walletID int64, limit, offset int) ([]*WalletTransaction, int, error) {
	txs := m.transactions[walletID]
	return txs, len(txs), nil
}

func (m *mockBillingRepo) ListTransactionsWithTypeTotal(_ context.Context, walletID int64, txType string, _, _ int) ([]*WalletTransaction, int, error) {
	if txType == "" {
		txs := m.transactions[walletID]
		return txs, len(txs), nil
	}
	var out []*WalletTransaction
	for _, tx := range m.transactions[walletID] {
		if tx != nil && string(tx.Type) == txType {
			out = append(out, tx)
		}
	}
	return out, len(out), nil
}

func (m *mockBillingRepo) CreatePayment(_ context.Context, p *Payment) error { return nil }
func (m *mockBillingRepo) GetPaymentByID(_ context.Context, id int64) (*Payment, error) {
	return nil, apperr.NotFound("payment")
}

func (m *mockBillingRepo) ListPlans(_ context.Context) ([]*Plan, error) {
	var list []*Plan
	for _, p := range m.plans {
		list = append(list, p)
	}
	return list, nil
}

func (m *mockBillingRepo) AdminListPlans(_ context.Context) ([]*Plan, error) {
	var list []*Plan
	for _, p := range m.plans {
		list = append(list, p)
	}
	return list, nil
}

func (m *mockBillingRepo) CreatePlan(_ context.Context, p *Plan) error {
	p.ID = 1
	return nil
}

func (m *mockBillingRepo) GetPlanByID(_ context.Context, id int64) (*Plan, error) {
	for _, p := range m.plans {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, apperr.NotFound("plan")
}

func (m *mockBillingRepo) GetDefaultPlan(_ context.Context) (*Plan, error) {
	for _, p := range m.plans {
		if p.IsDefault {
			return p, nil
		}
	}
	for _, p := range m.plans {
		return p, nil
	}
	return nil, apperr.NotFound("plan")
}

func (m *mockBillingRepo) UpdatePlan(_ context.Context, p *Plan) error {
	m.plans[p.Slug] = p
	return nil
}

func (m *mockBillingRepo) TogglePlanActive(_ context.Context, id int64) error {
	for _, p := range m.plans {
		if p.ID == id {
			p.IsActive = !p.IsActive
			return nil
		}
	}
	return nil
}

func (m *mockBillingRepo) SetDefaultPlan(_ context.Context, id int64) error {
	for _, p := range m.plans {
		p.IsDefault = (p.ID == id)
	}
	return nil
}

func (m *mockBillingRepo) DeletePlan(_ context.Context, id int64) error {
	for k, p := range m.plans {
		if p.ID == id {
			delete(m.plans, k)
			return nil
		}
	}
	return nil
}

func (m *mockBillingRepo) GetPlanBySlug(_ context.Context, slug string) (*Plan, error) {
	p, ok := m.plans[slug]
	if !ok {
		return nil, apperr.NotFound("plan")
	}
	return p, nil
}

func (m *mockBillingRepo) CreateSubscription(_ context.Context, sub *Subscription) error {
	sub.ID = m.nextID
	m.nextID++
	m.subscriptions[sub.UserID] = sub
	return nil
}

func (m *mockBillingRepo) GetActiveSubscription(_ context.Context, userID int64) (*Subscription, error) {
	sub, ok := m.subscriptions[userID]
	if !ok {
		return nil, apperr.NotFound("subscription")
	}
	return sub, nil
}

func (m *mockBillingRepo) GetActiveSubscriptionByOrg(_ context.Context, orgID int64) (*Subscription, error) {
	for _, sub := range m.subscriptions {
		if sub.OrganizationID != nil && *sub.OrganizationID == orgID && sub.Status == SubActive {
			return sub, nil
		}
	}
	return nil, apperr.NotFound("subscription")
}

func (m *mockBillingRepo) ListDueSubscriptionsForRenewal(_ context.Context, _ time.Time) ([]*Subscription, error) {
	var list []*Subscription
	for _, sub := range m.subscriptions {
		if sub.AutoRenew && sub.Status == SubActive {
			list = append(list, sub)
		}
	}
	return list, nil
}

func (m *mockBillingRepo) UpdateSubscriptionStatus(_ context.Context, id int64, status SubscriptionStatus, renewalAttempts int) error {
	for _, sub := range m.subscriptions {
		if sub.ID == id {
			sub.Status = status
			sub.RenewalAttempts = renewalAttempts
			return nil
		}
	}
	return nil
}

func (m *mockBillingRepo) RenewSubscription(_ context.Context, subID int64, walletID int64, cost money.Amount, newExpiresAt time.Time, _ string) error {
	for _, sub := range m.subscriptions {
		if sub.ID == subID {
			sub.ExpiresAt = newExpiresAt
			sub.Status = SubActive
			sub.RenewalAttempts = 0
			return nil
		}
	}
	return nil
}

func (m *mockBillingRepo) CheckEntitlement(_ context.Context, userID int64, featureKey string) (bool, string, error) {
	sub, ok := m.subscriptions[userID]
	if !ok || sub.Status != SubActive {
		return false, "", nil
	}
	for _, plan := range m.plans {
		if plan.ID == sub.PlanID || plan.Slug == "pro" {
			if plan.Features == nil {
				return false, "", nil
			}
			val, ok := plan.Features[featureKey]
			return ok, val, nil
		}
	}
	return false, "", nil
}

func (m *mockBillingRepo) CheckOrgEntitlement(_ context.Context, orgID, userID int64, featureKey string) (bool, error) {
	for _, sub := range m.subscriptions {
		if (sub.UserID == userID || (sub.OrganizationID != nil && *sub.OrganizationID == orgID)) && sub.Status == SubActive {
			for _, plan := range m.plans {
				if plan.ID == sub.PlanID {
					return plan.HasFeature(featureKey), nil
				}
			}
			return true, nil
		}
	}
	return false, nil
}

func (m *mockBillingRepo) GetSetting(_ context.Context, key string) (string, error) {
	if m.settings != nil {
		return m.settings[key], nil
	}
	return "", nil
}

func (m *mockBillingRepo) CreateInvoice(_ context.Context, inv *Invoice) error {
	inv.ID = m.nextID
	m.nextID++
	m.invoices[inv.ID] = inv
	return nil
}

func (m *mockBillingRepo) GetInvoiceByID(_ context.Context, id int64) (*Invoice, error) {
	inv, ok := m.invoices[id]
	if !ok {
		return nil, apperr.NotFound("invoice")
	}
	return inv, nil
}

func (m *mockBillingRepo) GetInvoiceByOrderID(_ context.Context, orderID int64) (*Invoice, error) {
	for _, inv := range m.invoices {
		if inv.OrderID != nil && *inv.OrderID == orderID {
			return inv, nil
		}
	}
	return nil, apperr.NotFound("invoice")
}

func (m *mockBillingRepo) UpdateInvoiceStatus(_ context.Context, id int64, status InvoiceStatus) error {
	if inv, ok := m.invoices[id]; ok {
		inv.Status = status
	}
	return nil
}

func (m *mockBillingRepo) ListInvoicesByOrg(_ context.Context, orgID int64, limit, offset int) ([]*Invoice, error) {
	var list []*Invoice
	for _, inv := range m.invoices {
		if inv.OrganizationID == orgID {
			list = append(list, inv)
		}
	}
	return list, nil
}

func (m *mockBillingRepo) ListInvoicesByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*Invoice, int, error) {
	list, err := m.ListInvoicesByOrg(ctx, orgID, limit, offset)
	return list, len(list), err
}

func (m *mockBillingRepo) AddPaymentMethod(_ context.Context, pm *UserPaymentMethod) error {
	pm.ID = m.nextID
	m.nextID++
	m.methods[pm.ID] = pm
	return nil
}

func (m *mockBillingRepo) GetPaymentMethodByID(_ context.Context, userID, id int64) (*UserPaymentMethod, error) {
	if pm, ok := m.methods[id]; ok && pm.UserID == userID {
		return pm, nil
	}
	return nil, apperr.NotFound("payment_method")
}

func (m *mockBillingRepo) ListPaymentMethods(_ context.Context, userID int64) ([]*UserPaymentMethod, error) {
	var list []*UserPaymentMethod
	for _, pm := range m.methods {
		if pm.UserID == userID {
			list = append(list, pm)
		}
	}
	return list, nil
}

func (m *mockBillingRepo) AdminGetFinanceStats(_ context.Context) (*AdminFinanceStats, error) {
	return &AdminFinanceStats{}, nil
}
