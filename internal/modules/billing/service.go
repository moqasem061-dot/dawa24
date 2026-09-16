package billing

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"log/slog"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Service manages digital wallets, ledger transactions, and plan entitlements.
type Service struct {
	repo Repository
	log  *slog.Logger
	// aiPlans propagates an organisation's entitlement to the AI Gateway when
	// its subscription changes. Optional; see ai_plan_sync.go.
	aiPlans AIPlanSync
}

// NewService creates a new billing service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// GetWallet retrieves or initializes a user's wallet with current balance.
func (s *Service) GetWallet(ctx context.Context, userID int64, currency string) (*Wallet, error) {
	return s.repo.GetOrCreateWallet(ctx, userID, currency)
}

// GetWalletByID retrieves a wallet by its database ID.
func (s *Service) GetWalletByID(ctx context.Context, id int64) (*Wallet, error) {
	return s.repo.GetWallet(ctx, id)
}

// ListWalletTransactions returns the paginated ledger for a wallet.
func (s *Service) ListWalletTransactions(ctx context.Context, walletID int64, limit, offset int) ([]*WalletTransaction, error) {
	return s.repo.ListTransactions(ctx, walletID, limit, offset)
}

// ListWalletTransactionsWithTotal returns the paginated ledger for a wallet with total count.
func (s *Service) ListWalletTransactionsWithTotal(ctx context.Context, walletID int64, limit, offset int) ([]*WalletTransaction, int, error) {
	return s.repo.ListTransactionsWithTotal(ctx, walletID, limit, offset)
}

// ListWalletTransactionsWithTypeTotal returns the paginated ledger for a
// wallet, optionally restricted to one transaction type (deposit, withdrawal,
// purchase...). Empty txType disables the type predicate.
func (s *Service) ListWalletTransactionsWithTypeTotal(ctx context.Context, walletID int64, txType string, limit, offset int) ([]*WalletTransaction, int, error) {
	return s.repo.ListTransactionsWithTypeTotal(ctx, walletID, txType, limit, offset)
}

// Deposit credits funds to a wallet.
func (s *Service) Deposit(
	ctx context.Context,
	userID int64,
	currency string,
	amount money.Amount,
	refType string,
	refID *int64,
	desc string,
) (*WalletTransaction, error) {
	if err := ValidateCreditAmount(amount); err != nil {
		return nil, err
	}

	wallet, err := s.repo.GetOrCreateWallet(ctx, userID, currency)
	if err != nil {
		return nil, err
	}

	tx, err := s.repo.RecordTransaction(ctx, wallet.ID, TxDeposit, amount, refType, refID, desc)
	if err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "wallet credited", "wallet_id", wallet.ID, "amount", amount.String(), "balance", tx.BalanceAfter.String())
	return tx, nil
}

// RequestDeposit initiates a funds deposit workflow, placing the request in pending status for admin review.
func (s *Service) RequestDeposit(
	ctx context.Context,
	userID int64,
	orgID *int64,
	currency string,
	amount money.Amount,
	method string,
	referenceNumber string,
	attachmentURL string,
	userNotes string,
) (*WalletDeposit, error) {
	return s.RequestDepositExtended(ctx, userID, orgID, currency, amount, method, referenceNumber, attachmentURL, userNotes, "", "", nil)
}

// EditPendingDeposit allows the user to update a deposit request as long as it remains in pending status.
func (s *Service) EditPendingDeposit(
	ctx context.Context,
	userID int64,
	depositID int64,
	amount money.Amount,
	method string,
	referenceNumber string,
	attachmentURL string,
	userNotes string,
) (*WalletDeposit, error) {
	if err := ValidateCreditAmount(amount); err != nil {
		return nil, err
	}
	if method == "" {
		return nil, apperr.Validation("payment_method.required", i18n.T("ar", "billing.val.method_required"), nil)
	}
	if referenceNumber == "" {
		return nil, apperr.Validation("reference_number.required", i18n.T("ar", "billing.val.ref_required"), nil)
	}

	existing, err := s.repo.GetDepositRequestByID(ctx, depositID)
	if err != nil {
		return nil, err
	}
	if existing.UserID != userID {
		return nil, apperr.Forbidden("deposit.not_owner", i18n.T("ar", "billing.err.not_owner"))
	}
	if existing.Status != DepositPending {
		return nil, apperr.Conflict("deposit.locked", i18n.T("ar", "billing.err.locked"))
	}

	existing.Amount = amount
	existing.PaymentMethod = method
	existing.ReferenceNumber = referenceNumber
	if attachmentURL != "" {
		existing.AttachmentURL = attachmentURL
	}
	existing.UserNotes = userNotes

	if err := s.repo.UpdatePendingDepositRequest(ctx, existing); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "wallet deposit updated by user", "deposit_id", depositID, "user_id", userID, "amount", amount.String())
	return existing, nil
}

// ListUserDeposits returns all deposit requests submitted by a user.
func (s *Service) ListUserDeposits(ctx context.Context, userID int64, limit, offset int) ([]*WalletDeposit, error) {
	return s.repo.ListDepositRequestsByUser(ctx, userID, limit, offset)
}

// ListUserDepositsWithStatus returns a user's deposit requests, optionally
// restricted to one status (pending, approved, rejected). Empty status
// disables the status predicate.
func (s *Service) ListUserDepositsWithStatus(ctx context.Context, userID int64, status string, limit, offset int) ([]*WalletDeposit, error) {
	return s.repo.ListDepositRequestsByUserWithStatus(ctx, userID, status, limit, offset)
}

// Withdraw debits funds from a wallet, rejecting overdrafts.
func (s *Service) Withdraw(
	ctx context.Context,
	userID int64,
	currency string,
	amount money.Amount,
	refType string,
	refID *int64,
	desc string,
) (*WalletTransaction, error) {
	if err := ValidateCreditAmount(amount); err != nil {
		return nil, err
	}

	wallet, err := s.repo.GetOrCreateWallet(ctx, userID, currency)
	if err != nil {
		return nil, err
	}

	if wallet.AvailableBalance.Minor() < amount.Minor() {
		return nil, apperr.Validation("wallet.insufficient_funds", "رصيد المحفظة المتاح غير كافٍ لإتمام هذه العملية.", nil)
	}

	negDelta, err := money.Zero.Sub(amount)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	tx, err := s.repo.RecordTransaction(ctx, wallet.ID, TxWithdrawal, negDelta, refType, refID, desc)
	if err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "wallet debited", "wallet_id", wallet.ID, "amount", amount.String(), "balance", tx.BalanceAfter.String())
	return tx, nil
}

// ListPlans returns all active subscription plans.
func (s *Service) ListPlans(ctx context.Context) ([]*Plan, error) {
	return s.repo.ListPlans(ctx)
}

// AdminListPlans returns all subscription plans (both active and inactive) for administrative management.
func (s *Service) AdminListPlans(ctx context.Context) ([]*Plan, error) {
	return s.repo.AdminListPlans(ctx)
}

// GetPlanByID retrieves a plan by ID.
func (s *Service) GetPlanByID(ctx context.Context, id int64) (*Plan, error) {
	return s.repo.GetPlanByID(ctx, id)
}

// GetPlanBySlug retrieves a plan by slug.
func (s *Service) GetPlanBySlug(ctx context.Context, slug string) (*Plan, error) {
	return s.repo.GetPlanBySlug(ctx, slug)
}

// GetDefaultPlan retrieves the default basic plan.
func (s *Service) GetDefaultPlan(ctx context.Context) (*Plan, error) {
	return s.repo.GetDefaultPlan(ctx)
}

// CreatePlan adds a subscription tier (admin).
func (s *Service) CreatePlan(ctx context.Context, p *Plan) (*Plan, error) {
	if p.Slug == "" || p.Name.IsEmpty() {
		return nil, apperr.Validation("plan.slug_required", "A plan slug and name are required.", nil)
	}
	if err := s.repo.CreatePlan(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// UpdatePlan updates an existing subscription tier.
func (s *Service) UpdatePlan(ctx context.Context, p *Plan) (*Plan, error) {
	if p.ID <= 0 || p.Name.IsEmpty() {
		return nil, apperr.Validation("plan.invalid", "Plan ID and name are required.", nil)
	}
	if err := s.repo.UpdatePlan(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// TogglePlanActive toggles the active state of a plan.
func (s *Service) TogglePlanActive(ctx context.Context, id int64) error {
	if id <= 0 {
		return apperr.Validation("plan.invalid", "Plan ID is required.", nil)
	}
	return s.repo.TogglePlanActive(ctx, id)
}

// SetDefaultPlan designates a plan as the system default tier.
func (s *Service) SetDefaultPlan(ctx context.Context, id int64) error {
	if id <= 0 {
		return apperr.Validation("plan.invalid", "Plan ID is required.", nil)
	}
	return s.repo.SetDefaultPlan(ctx, id)
}

// DeletePlan removes a plan from the system.
func (s *Service) DeletePlan(ctx context.Context, id int64) error {
	if id <= 0 {
		return apperr.Validation("plan.invalid", "Plan ID is required.", nil)
	}
	return s.repo.DeletePlan(ctx, id)
}

// GetActiveSubscription returns active subscription for a user.
func (s *Service) GetActiveSubscription(ctx context.Context, userID int64) (*Subscription, error) {
	return s.repo.GetActiveSubscription(ctx, userID)
}

// GetActiveSubscriptionByOrg returns active subscription for an organization.
func (s *Service) GetActiveSubscriptionByOrg(ctx context.Context, orgID int64) (*Subscription, error) {
	return s.repo.GetActiveSubscriptionByOrg(ctx, orgID)
}

// GetEffectivePlan resolves the active plan for an organization or user, falling back to the system default plan.
func (s *Service) GetEffectivePlan(ctx context.Context, userID int64, orgID *int64) (*Plan, error) {
	if orgID != nil && *orgID > 0 {
		if sub, err := s.repo.GetActiveSubscriptionByOrg(ctx, *orgID); err == nil && sub != nil {
			if plan, err := s.repo.GetPlanByID(ctx, sub.PlanID); err == nil && plan != nil {
				return plan, nil
			}
		}
	}
	if userID > 0 {
		if sub, err := s.repo.GetActiveSubscription(ctx, userID); err == nil && sub != nil {
			if plan, err := s.repo.GetPlanByID(ctx, sub.PlanID); err == nil && plan != nil {
				return plan, nil
			}
		}
	}
	if plan, err := s.repo.GetDefaultPlan(ctx); err == nil && plan != nil {
		return plan, nil
	}
	return s.repo.GetPlanBySlug(ctx, "basic")
}

// GetVendorPaymentStats returns aggregated KPIs for vendor payments dashboard.
func (s *Service) GetVendorPaymentStats(ctx context.Context, orgID int64) (*VendorPaymentStats, error) {
	return s.repo.GetVendorPaymentStats(ctx, orgID)
}

// RecordInvoicePayment records a payment against an invoice and updates remaining balances and status.
func (s *Service) RecordInvoicePayment(ctx context.Context, req RecordInvoicePaymentRequest) (*Payment, error) {
	return s.repo.RecordInvoicePayment(ctx, req)
}

// ListDetailedPayments returns enriched payment records with invoice and customer metadata.
func (s *Service) ListDetailedPayments(ctx context.Context, filter PaymentFilter) ([]*AdminPaymentView, int, error) {
	return s.repo.AdminListDetailedPayments(ctx, filter)
}

// ListDetailedInvoices returns enriched invoice records for administration or vendor dashboards.
func (s *Service) ListDetailedInvoices(ctx context.Context, filter InvoiceFilter) ([]*AdminInvoiceView, int, error) {
	return s.repo.AdminListDetailedInvoices(ctx, filter)
}

// ListVendorCustomerOrgs returns the distinct customer organizations associated with a vendor's transactions.
func (s *Service) ListVendorCustomerOrgs(ctx context.Context, vendorOrgID int64) ([]*CustomerOrgSummary, error) {
	if vendorOrgID <= 0 {
		return nil, nil
	}
	return s.repo.ListVendorCustomerOrgs(ctx, vendorOrgID)
}
