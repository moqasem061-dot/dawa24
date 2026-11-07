package pages

import (
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// VendorOrderOption represents a candidate order for invoice generation.
type VendorOrderOption struct {
	ID           int64        `json:"id"`
	OrderNumber  string       `json:"order_number"`
	CustomerName string       `json:"customer_name"`
	TotalAmount  money.Amount `json:"total_amount"`
	Subtotal     money.Amount `json:"subtotal"`
	TaxAmount    money.Amount `json:"tax_amount"`
	CreatedAt    time.Time    `json:"created_at"`
}

// WalletData is the /wallet view model: balance, ledger and payment methods.
type WalletData struct {
	Balance        money.Amount
	Currency       string
	Transactions   []*billing.WalletTransaction
	PaymentMethods []*billing.UserPaymentMethod
}

// InvoicesData is the /invoices view model.
type InvoicesData struct {
	Invoices         []*billing.AdminInvoiceView
	RawInvoices      []*billing.Invoice
	Search           string
	StatusFilter     string
	DateFrom         string
	DateTo           string
	SortBy           string
	SortOrder        string
	Branches         []*org.Branch
	SelectedBranchID int64
	VendorOrders     []VendorOrderOption
	IsVendor         bool
	Page             int
	PerPage          int
	TotalCount       int
}
