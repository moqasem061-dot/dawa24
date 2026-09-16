package billing

import (
	"strconv"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// TransactionType identifies categories of wallet balance changes.
type TransactionType string

const (
	TxDeposit     TransactionType = "deposit"
	TxWithdrawal  TransactionType = "withdrawal"
	TxPurchase    TransactionType = "purchase"
	TxRefund      TransactionType = "refund"
	TxBonus       TransactionType = "bonus"
	TxPenalty     TransactionType = "penalty"
	TxTransferIn  TransactionType = "transfer_in"
	TxTransferOut TransactionType = "transfer_out"
	TxAdjustment  TransactionType = "adjustment"
)

// SubscriptionStatus tracks subscription validity.
type SubscriptionStatus string

const (
	SubActive    SubscriptionStatus = "active"
	SubTrialing  SubscriptionStatus = "trialing"
	SubPastDue   SubscriptionStatus = "past_due"
	SubCancelled SubscriptionStatus = "cancelled"
	SubExpired   SubscriptionStatus = "expired"
)

// Wallet represents a tenant or user digital balance account.
type Wallet struct {
	ID                int64        `json:"id"`
	PublicID          string       `json:"public_id"`
	UserID            int64        `json:"user_id"`
	OrganizationID    *int64       `json:"organization_id,omitempty"`
	Currency          string       `json:"currency"`
	Balance           money.Amount `json:"balance"`            // Total balance computed from ledger
	PendingWithdrawal money.Amount `json:"pending_withdrawal"` // Sum of pending withdrawal requests
	AvailableBalance  money.Amount `json:"available_balance"`  // Balance - PendingWithdrawal
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

// Available returns the net usable balance (total balance minus pending withdrawals).
func (w *Wallet) Available() money.Amount {
	if w == nil {
		return money.Zero
	}
	return w.AvailableBalance
}

// Pending returns the pending withdrawal balance.
func (w *Wallet) Pending() money.Amount {
	if w == nil {
		return money.Zero
	}
	return w.PendingWithdrawal
}

// WalletTransaction is an immutable append-only ledger row.
type WalletTransaction struct {
	ID                    int64           `json:"id"`
	WalletID              int64           `json:"wallet_id"`
	Type                  TransactionType `json:"type"`
	Amount                money.Amount    `json:"amount"` // Signed: positive = credit, negative = debit
	BalanceAfter          money.Amount    `json:"balance_after"`
	ReferenceType         string          `json:"reference_type,omitempty"`
	ReferenceID           *int64          `json:"reference_id,omitempty"`
	Description           string          `json:"description,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	ReversesTransactionID *int64          `json:"reverses_transaction_id,omitempty"`
	RefundedBy            *int64          `json:"refunded_by,omitempty"`
	RefundedAt            *time.Time      `json:"refunded_at,omitempty"`
}

// DepositStatus tracks the approval workflow lifecycle of a wallet deposit.
type DepositStatus string

const (
	DepositPending  DepositStatus = "pending"
	DepositApproved DepositStatus = "approved"
	DepositRejected DepositStatus = "rejected"
	DepositRefunded DepositStatus = "refunded"
)

// WalletDeposit records a user deposit request subject to administrative approval.
type WalletDeposit struct {
	ID                    int64         `json:"id"`
	PublicID              string        `json:"public_id"`
	WalletID              int64         `json:"wallet_id"`
	UserID                int64         `json:"user_id"`
	OrganizationID        *int64        `json:"organization_id,omitempty"`
	Amount                money.Amount  `json:"amount"`
	Currency              string        `json:"currency"`
	PaymentMethod         string        `json:"payment_method"`
	ReferenceNumber       string        `json:"reference_number"`
	PlatformMethodID      string        `json:"platform_method_id,omitempty"`
	SenderAccount         string        `json:"sender_account,omitempty"`
	SenderPaymentMethodID *int64        `json:"sender_payment_method_id,omitempty"`
	AttachmentURL         string        `json:"attachment_url,omitempty"`
	UserNotes             string        `json:"user_notes,omitempty"`
	Status                DepositStatus `json:"status"`
	RejectionReason       string        `json:"rejection_reason,omitempty"`
	ReviewedBy            *int64        `json:"reviewed_by,omitempty"`
	ReviewedAt            *time.Time    `json:"reviewed_at,omitempty"`
	TransactionID         *int64        `json:"transaction_id,omitempty"`
	RefundedBy            *int64        `json:"refunded_by,omitempty"`
	RefundedAt            *time.Time    `json:"refunded_at,omitempty"`
	RefundTransactionID   *int64        `json:"refund_transaction_id,omitempty"`
	CreatedAt             time.Time     `json:"created_at"`
	UpdatedAt             time.Time     `json:"updated_at"`
}

// CanEdit reports whether the deposit is still pending and eligible for user modification.
func (d *WalletDeposit) CanEdit() bool {
	return d != nil && d.Status == DepositPending
}

// Payment records a monetary transaction via a payment method.
type Payment struct {
	ID                   int64        `json:"id"`
	PublicID             string       `json:"public_id"`
	PaymentIntegrationID *int64       `json:"payment_integration_id,omitempty"`
	InvoiceID            *int64       `json:"invoice_id,omitempty"`
	InvoiceNumber        string       `json:"invoice_number,omitempty"`
	CustomerName         string       `json:"customer_name,omitempty"`
	OrderID              *int64       `json:"order_id,omitempty"`
	UserID               int64        `json:"user_id"`
	OrganizationID       *int64       `json:"organization_id,omitempty"`
	Amount               money.Amount `json:"amount"`
	Method               string       `json:"method"`
	Status               string       `json:"status"`
	TransactionID        string       `json:"transaction_id,omitempty"`
	ReferenceNumber      string       `json:"reference_number,omitempty"`
	PaidAt               *time.Time   `json:"paid_at,omitempty"`
	CreatedAt            time.Time    `json:"created_at"`
	UpdatedAt            time.Time    `json:"updated_at"`
}

// Plan represents a subscription tier with billing parameters, limits, and AI Gateway plan linkage.
type Plan struct {
	ID               int64             `json:"id"`
	PublicID         string            `json:"public_id"`
	Slug             string            `json:"slug"`
	Name             i18n.Text         `json:"name"`
	Description      i18n.Text         `json:"description,omitempty"`
	PriceMonth       money.Amount      `json:"price_month"`
	PriceYear        money.Amount      `json:"price_year"`
	DurationDays     int               `json:"duration_days"`
	MaxUsers         *int              `json:"max_users,omitempty"`
	MaxLoginSessions int               `json:"max_login_sessions"`
	MaxDevices       int               `json:"max_devices"`
	AIPlanID         string            `json:"ai_plan_id"`
	IsDefault        bool              `json:"is_default"`
	IsActive         bool              `json:"is_active"`
	Features         map[string]string `json:"features,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// Subscription feature gate keys for platform access control.
const (
	FeatureMarketDiscounts      = "feature_market_discounts"
	FeatureCompareTool          = "feature_compare_tool"
	FeatureBulkImport           = "bulk_import"
	FeatureAnalytics            = "analytics"
	FeatureMaxCompareFiles      = "max_compare_files"
	FeatureCompareRetentionDays = "compare_file_retention_days"
)

// HasFeature reports whether the plan grants access to the specified feature key.
func (p *Plan) HasFeature(key string) bool {
	if p == nil || p.Features == nil {
		return false
	}
	v, ok := p.Features[key]
	if !ok {
		return false
	}
	return v == "true" || v == "1" || v == "enabled"
}

// GetMaxCompareFiles returns the max active supplier files allowed for price comparison.
// Default is 10 if not set. 0 means unlimited.
func (p *Plan) GetMaxCompareFiles() int {
	if p == nil || p.Features == nil {
		return 10
	}
	if v, ok := p.Features[FeatureMaxCompareFiles]; ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	if v, ok := p.Features["max_compare_files"]; ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return 10
}

// GetCompareRetentionDays returns the number of days compare files are retained before auto-deletion.
// Default is 30 days if not set. 0 means unlimited (files are never auto-deleted).
func (p *Plan) GetCompareRetentionDays() int {
	if p == nil || p.Features == nil {
		return 30
	}
	if v, ok := p.Features[FeatureCompareRetentionDays]; ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	if v, ok := p.Features["compare_file_retention_days"]; ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return 30
}

// Subscription represents an active plan subscription (unifying legacy D7 systems).
type Subscription struct {
	ID              int64              `json:"id"`
	PublicID        string             `json:"public_id"`
	UserID          int64              `json:"user_id"`
	OrganizationID  *int64             `json:"organization_id,omitempty"`
	PlanID          int64              `json:"plan_id"`
	Status          SubscriptionStatus `json:"status"`
	BillingCycle    string             `json:"billing_cycle"` // "monthly" or "annual"
	AutoRenew       bool               `json:"auto_renew"`
	StartsAt        time.Time          `json:"starts_at"`
	ExpiresAt       time.Time          `json:"expires_at"`
	LastRenewedAt   *time.Time         `json:"last_renewed_at,omitempty"`
	RenewalAttempts int                `json:"renewal_attempts"`
	SourceSystem    string             `json:"source_system"`
	SourceID        *int64             `json:"source_id,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// IsActiveAt checks whether the subscription is active at a given moment in time.
func (s *Subscription) IsActiveAt(now time.Time) bool {
	if s == nil || s.Status != SubActive {
		return false
	}
	return s.StartsAt.Before(now) && s.ExpiresAt.After(now)
}

// Validate ensures non-negative amounts for deposit and credit operations.
func ValidateCreditAmount(a money.Amount) error {
	if a.IsZero() || a.IsNegative() {
		return apperr.Validation("amount.invalid", "Amount must be strictly positive.", nil)
	}
	return nil
}

// InvoiceStatus tracks invoice lifecycle states.
type InvoiceStatus string

const (
	InvoiceDraft         InvoiceStatus = "draft"
	InvoiceIssued        InvoiceStatus = "issued"
	InvoicePartiallyPaid InvoiceStatus = "partially_paid"
	InvoicePaid          InvoiceStatus = "paid"
	InvoiceOverdue       InvoiceStatus = "overdue"
	InvoiceCancelled     InvoiceStatus = "cancelled"
)

// Invoice represents a B2B commercial tax invoice.
type Invoice struct {
	ID              int64         `json:"id"`
	PublicID        string        `json:"public_id"`
	OrganizationID  int64         `json:"organization_id"`
	CustomerOrgID   *int64        `json:"customer_org_id,omitempty"`
	OrderID         *int64        `json:"order_id,omitempty"`
	InvoiceNumber   string        `json:"invoice_number"`
	IssueDate       time.Time     `json:"issue_date"`
	DueDate         time.Time     `json:"due_date"`
	Subtotal        money.Amount  `json:"subtotal"`
	TaxAmount       money.Amount  `json:"tax_amount"`
	DiscountAmount  money.Amount  `json:"discount_amount"`
	TotalAmount     money.Amount  `json:"total_amount"`
	PaidAmount      money.Amount  `json:"paid_amount"`
	RemainingAmount money.Amount  `json:"remaining_amount"`
	Status          InvoiceStatus `json:"status"`
	PaymentMethod   string        `json:"payment_method"`
	Notes           string        `json:"notes,omitempty"`
	Lines           []InvoiceLine `json:"lines,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

// InvoiceLine is a line item on an invoice.
type InvoiceLine struct {
	ID          int64        `json:"id"`
	InvoiceID   int64        `json:"invoice_id"`
	ProductID   *int64       `json:"product_id,omitempty"`
	Description string       `json:"description"`
	Quantity    int          `json:"quantity"`
	UnitPrice   money.Amount `json:"unit_price"`
	TotalPrice  money.Amount `json:"total_price"`
}

// PlatformPaymentMethod represents a payment channel configured for the platform.
type PlatformPaymentMethod struct {
	ID                string    `json:"id"`                  // e.g. "bank_transfer", "instapay", "card", "vodafone_cash"
	Name              i18n.Text `json:"name"`                // Arabic & English name
	ProviderType      string    `json:"provider_type"`       // "bank", "instapay", "wallet", "card", "cash"
	Description       i18n.Text `json:"description"`         // Arabic & English guidelines
	AccountName       string    `json:"account_name"`        // Corporate account holder
	BankName          string    `json:"bank_name"`           // Official bank name
	AccountNumber     string    `json:"account_number"`      // Bank account number
	IBAN              string    `json:"iban"`                // Official IBAN
	SwiftCode         string    `json:"swift_code"`          // SWIFT / BIC code
	BranchName        string    `json:"branch_name"`         // Bank branch
	InstaPayHandle    string    `json:"instapay_handle"`     // IPA address e.g. dawa24@instapay
	PhoneNumber       string    `json:"phone_number"`        // Phone number for wallets / InstaPay
	IsActive          bool      `json:"is_active"`           // Globally active or disabled
	IsDepositEnabled  bool      `json:"is_deposit_enabled"`  // Allowed for wallet deposit
	IsCheckoutEnabled bool      `json:"is_checkout_enabled"` // Allowed for checkout orders
	DisplayOrder      int       `json:"display_order"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// AdminWalletView represents an enriched wallet view for administration.
type AdminWalletView struct {
	ID                int64        `json:"id"`
	PublicID          string       `json:"public_id"`
	UserID            int64        `json:"user_id"`
	UserName          string       `json:"user_name"`
	UserEmail         string       `json:"user_email"`
	UserPhone         string       `json:"user_phone"`
	OrganizationID    *int64       `json:"organization_id,omitempty"`
	OrganizationName  string       `json:"organization_name"`
	OrganizationType  string       `json:"organization_type"` // "customer" | "vendor" | "supplier"
	Currency          string       `json:"currency"`
	Balance           money.Amount `json:"balance"`
	PendingWithdrawal money.Amount `json:"pending_withdrawal"`
	AvailableBalance  money.Amount `json:"available_balance"`
	CreatedAt         time.Time    `json:"created_at"`
	TransactionsCount int          `json:"transactions_count"`
}

// AdminWalletTransactionView represents an enriched ledger entry.
type AdminWalletTransactionView struct {
	ID                    int64           `json:"id"`
	WalletID              int64           `json:"wallet_id"`
	Type                  TransactionType `json:"type"`
	Amount                money.Amount    `json:"amount"`
	BalanceAfter          money.Amount    `json:"balance_after"`
	ReferenceType         string          `json:"reference_type,omitempty"`
	ReferenceID           *int64          `json:"reference_id,omitempty"`
	Description           string          `json:"description,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	UserID                int64           `json:"user_id"`
	UserName              string          `json:"user_name"`
	UserEmail             string          `json:"user_email"`
	OrganizationName      string          `json:"organization_name"`
	OrganizationType      string          `json:"organization_type"`
	ReversesTransactionID *int64          `json:"reverses_transaction_id,omitempty"`
	IsRefunded            bool            `json:"is_refunded"`
	RefundedByID          *int64          `json:"refunded_by_id,omitempty"`
	RefundedByName        string          `json:"refunded_by_name,omitempty"`
	RefundedAt            *time.Time      `json:"refunded_at,omitempty"`
	RefundTransactionID   *int64          `json:"refund_transaction_id,omitempty"`
}
