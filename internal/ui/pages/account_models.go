package pages

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
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

// TxLabel maps a wallet transaction type onto an Arabic label.
func TxLabel(t billing.TransactionType) string {
	switch t {
	case billing.TxDeposit:
		return i18n.T("ar", "tx.deposit")
	case billing.TxWithdrawal:
		return i18n.T("ar", "tx.withdrawal")
	case billing.TxPurchase:
		return i18n.T("ar", "tx.purchase")
	case billing.TxRefund:
		return i18n.T("ar", "tx.refund")
	case billing.TxBonus:
		return i18n.T("ar", "tx.bonus")
	case billing.TxPenalty:
		return i18n.T("ar", "tx.penalty")
	case billing.TxTransferIn:
		return i18n.T("ar", "tx.transfer_in")
	case billing.TxTransferOut:
		return i18n.T("ar", "tx.transfer_out")
	case billing.TxAdjustment:
		return i18n.T("ar", "tx.adjustment")
	default:
		return string(t)
	}
}

// InvoiceLabel maps an invoice status onto an Arabic label.
func InvoiceLabel(s billing.InvoiceStatus) string {
	switch s {
	case billing.InvoiceDraft:
		return i18n.T("ar", "invoice.status_draft")
	case billing.InvoiceIssued:
		return i18n.T("ar", "invoice.status_issued")
	case billing.InvoicePaid:
		return i18n.T("ar", "invoice.status_paid")
	case billing.InvoiceOverdue:
		return i18n.T("ar", "invoice.status_overdue")
	case billing.InvoiceCancelled:
		return i18n.T("ar", "invoice.status_cancelled")
	default:
		return string(s)
	}
}

// InvoiceBadgeClass maps an invoice status onto a badge colour class.
func InvoiceBadgeClass(s billing.InvoiceStatus) string {
	switch s {
	case billing.InvoicePaid:
		return "badge-emerald"
	case billing.InvoiceIssued:
		return "badge-amber"
	case billing.InvoiceOverdue:
		return "badge-rose"
	case billing.InvoiceCancelled:
		return "badge-rose"
	default:
		return "badge-amber"
	}
}

// msgStatusLabel maps a contact-message status onto an Arabic label.
func msgStatusLabel(s string) string {
	switch s {
	case "resolved":
		return i18n.TDefault("w4_ui.s_179_179")
	case "read":
		return i18n.TDefault("w4_ui.s_180_180")
	default:
		return i18n.TDefault("w4_ui.s_181_181")
	}
}

// msgStatusBadge maps a contact-message status onto a badge colour class.
func msgStatusBadge(s string) string {
	switch s {
	case "resolved":
		return "badge-emerald"
	case "read":
		return "badge-amber"
	default:
		return "badge-rose"
	}
}
