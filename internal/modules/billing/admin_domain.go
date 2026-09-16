package billing

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// AdminInvoiceView represents an enriched B2B invoice view for administration.
type AdminInvoiceView struct {
	ID              int64         `json:"id"`
	PublicID        string        `json:"public_id"`
	OrganizationID  int64         `json:"organization_id"`
	VendorName      string        `json:"vendor_name"`
	CustomerOrgID   *int64        `json:"customer_org_id,omitempty"`
	CustomerName    string        `json:"customer_name"`
	OrderID         *int64        `json:"order_id,omitempty"`
	OrderNumber     string        `json:"order_number,omitempty"`
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
	CreatedAt       time.Time     `json:"created_at"`
}

// AdminPaymentView represents an enriched payment record for administration.
type AdminPaymentView struct {
	ID                   int64        `json:"id"`
	PublicID             string       `json:"public_id"`
	PaymentIntegrationID *int64       `json:"payment_integration_id,omitempty"`
	InvoiceID            *int64       `json:"invoice_id,omitempty"`
	InvoiceNumber        string       `json:"invoice_number,omitempty"`
	CustomerName         string       `json:"customer_name,omitempty"`
	OrderID              *int64       `json:"order_id,omitempty"`
	OrderNumber          string       `json:"order_number,omitempty"`
	UserID               int64        `json:"user_id"`
	UserName             string       `json:"user_name"`
	OrganizationID       *int64       `json:"organization_id,omitempty"`
	OrganizationName     string       `json:"organization_name"`
	Amount               money.Amount `json:"amount"`
	Method               string       `json:"method"`
	Status               string       `json:"status"`
	TransactionID        string       `json:"transaction_id,omitempty"`
	ReferenceNumber      string       `json:"reference_number,omitempty"`
	Notes                string       `json:"notes,omitempty"`
	PaidAt               *time.Time   `json:"paid_at,omitempty"`
	CreatedAt            time.Time    `json:"created_at"`
}

// AdminWalletDepositView represents an enriched deposit request record for admin audit and review.
type AdminWalletDepositView struct {
	ID               int64         `json:"id"`
	PublicID         string        `json:"public_id"`
	WalletID         int64         `json:"wallet_id"`
	UserID           int64         `json:"user_id"`
	UserName         string        `json:"user_name"`
	UserEmail        string        `json:"user_email"`
	UserPhone        string        `json:"user_phone"`
	OrganizationID   *int64        `json:"organization_id,omitempty"`
	OrganizationName string        `json:"organization_name"`
	OrganizationType string        `json:"organization_type"`
	Amount           money.Amount  `json:"amount"`
	Currency         string        `json:"currency"`
	PaymentMethod    string        `json:"payment_method"`
	ReferenceNumber  string        `json:"reference_number"`
	AttachmentURL    string        `json:"attachment_url,omitempty"`
	UserNotes        string        `json:"user_notes,omitempty"`
	Status           DepositStatus `json:"status"`
	RejectionReason  string        `json:"rejection_reason,omitempty"`
	ReviewedBy       *int64        `json:"reviewed_by,omitempty"`
	ReviewerName     string        `json:"reviewer_name,omitempty"`
	ReviewedAt       *time.Time    `json:"reviewed_at,omitempty"`
	TransactionID       *int64        `json:"transaction_id,omitempty"`
	IsRefunded          bool          `json:"is_refunded"`
	RefundedBy          *int64        `json:"refunded_by,omitempty"`
	RefundedByName      string        `json:"refunded_by_name,omitempty"`
	RefundedAt          *time.Time    `json:"refunded_at,omitempty"`
	RefundTransactionID *int64        `json:"refund_transaction_id,omitempty"`
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`
}

// WalletFilter specifies parameters for querying wallets.
type WalletFilter struct {
	OrganizationID int64
	Search         string
	Type           string // "customer", "vendor", ""
	Limit          int
	Offset         int
}

// DepositFilter specifies parameters for querying wallet deposit requests.
type DepositFilter struct {
	UserID         int64
	WalletID       int64
	OrganizationID int64
	Status         string // "pending", "approved", "rejected", ""
	PaymentMethod  string
	Search         string
	Limit          int
	Offset         int
}

// TransactionFilter specifies parameters for querying wallet ledger records.
type TransactionFilter struct {
	WalletID       int64
	OrganizationID int64
	Type           string
	Search         string
	Limit          int
	Offset         int
}

// InvoiceFilter specifies parameters for querying invoices.
type InvoiceFilter struct {
	Search         string
	Status         string
	OrganizationID *int64
	CustomerOrgID  *int64
	BranchID       *int64
	DateFrom       string
	DateTo         string
	SortBy         string
	SortOrder      string
	Limit          int
	Offset         int
}

// PaymentFilter specifies parameters for querying payments.
type PaymentFilter struct {
	Search         string
	CustomerSearch string
	Method         string
	Status         string
	OrganizationID *int64
	InvoiceID      *int64
	// DateFrom/DateTo filter COALESCE(paid_at, created_at) by day (YYYY-MM-DD).
	DateFrom string
	DateTo   string
	Limit    int
	Offset   int
}

// CustomerOrgSummary provides basic details for autocomplete customer filtering.
type CustomerOrgSummary struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

// VendorPaymentStats contains KPI summaries for vendor payments dashboard.
type VendorPaymentStats struct {
	TotalCount  int          `json:"total_count"`
	TotalAmount money.Amount `json:"total_amount"`
	TodayAmount money.Amount `json:"today_amount"`
	MonthAmount money.Amount `json:"month_amount"`
}

// RecordInvoicePaymentRequest contains payload to register a payment against an invoice.
type RecordInvoicePaymentRequest struct {
	InvoiceID       int64        `json:"invoice_id"`
	OrganizationID  int64        `json:"organization_id"`
	UserID          int64        `json:"user_id"`
	Amount          money.Amount `json:"amount"`
	Method          string       `json:"method"`
	ReferenceNumber string       `json:"reference_number"`
	Notes           string       `json:"notes"`
	PaidAt          *time.Time   `json:"paid_at,omitempty"`
}

// PrintableOrgInfo holds commercial and tax registration details of a party on an invoice.
type PrintableOrgInfo struct {
	OrganizationID     int64  `json:"organization_id"`
	OrganizationNumber string `json:"organization_number,omitempty"` // رقم المنظمة (كود الحساب المسجل لدى المورد)
	DisplayName        string `json:"display_name"`
	LegalName          string `json:"legal_name"`
	LogoURL            string `json:"logo_url,omitempty"`
	TaxNumber          string `json:"tax_number"`          // البطاقة الضريبية (ETA Tax Registration)
	CommercialRegister string `json:"commercial_register"` // السجل التجاري
	PharmacistLicense  string `json:"pharmacist_license"`  // ترخيص مزاولة المهنة / الصيدلية
	Phone              string `json:"phone"`
	Address            string `json:"address"`
	City               string `json:"city"`
	Governorate        string `json:"governorate"`
}

// PrintableInvoiceLine represents an itemized line on the official printed invoice.
type PrintableInvoiceLine struct {
	Index           int          `json:"index"`
	ProductID       *int64       `json:"product_id,omitempty"`
	ItemName        string       `json:"item_name"`
	SKU             string       `json:"sku,omitempty"`
	BatchNumber     string       `json:"batch_number,omitempty"` // رقم التشغيلة
	ExpiryDate      string       `json:"expiry_date,omitempty"`  // تاريخ انتهاء الصلاحية
	Quantity        int          `json:"quantity"`
	UnitPrice       money.Amount `json:"unit_price"`       // سعر الجمهور / الوحدة الرسمي
	DiscountPercent float64      `json:"discount_percent"` // نسبة الخصم التجاري %
	NetUnitPrice    money.Amount `json:"net_unit_price"`   // سعر الوحدة الصافي بعد الخصم
	TotalPrice      money.Amount `json:"total_price"`      // الإجمالي للسطر
	IsExempt        bool         `json:"is_exempt"`        // معفى من ضريبة القيمة المضافة (أدوية بشرية قانون 67/2016)
}

// PrintableInvoiceData holds the full payload required to render an official Egyptian tax invoice (Thermal POS / A4).
type PrintableInvoiceData struct {
	InvoiceID      int64                   `json:"invoice_id"`
	PublicID       string                  `json:"public_id"`
	InvoiceNumber  string                  `json:"invoice_number"`
	OrderID        *int64                  `json:"order_id,omitempty"`
	OrderNumber    string                  `json:"order_number,omitempty"`
	IssueDate      time.Time               `json:"issue_date"`
	DueDate        time.Time               `json:"due_date"`
	Vendor         PrintableOrgInfo        `json:"vendor"`
	Customer       PrintableOrgInfo        `json:"customer"`
	Lines          []*PrintableInvoiceLine `json:"lines"`
	Subtotal       money.Amount            `json:"subtotal"`         // الإجمالي قبل الخصم
	TotalDiscount  money.Amount            `json:"total_discount"`   // إجمالي الخصم التجاري
	TaxableAmount  money.Amount            `json:"taxable_amount"`   // الوعاء الخاضع للضريبة
	VATAmtExempt   money.Amount            `json:"vat_amt_exempt"`   // ضريبة 0% (أدوية بشرية معفاة)
	VATAmtStandard money.Amount            `json:"vat_amt_standard"` // ضريبة 14% (مستلزمات ومستحضرات)
	TotalTax       money.Amount            `json:"total_tax"`        // إجمالي ضريبة القيمة المضافة
	TotalAmount    money.Amount            `json:"total_amount"`     // الصافي الإجمالي المطلوب سداده
	Status         InvoiceStatus           `json:"status"`
	PaymentMethod  string                  `json:"payment_method"`
	PaymentStatus  string                  `json:"payment_status"`
	DeliveryCode   string                  `json:"delivery_code,omitempty"`
	TrackingNumber string                  `json:"tracking_number,omitempty"`
	Notes          string                  `json:"notes,omitempty"`
	QRCodeData     string                  `json:"qr_code_data"`
}

// WithdrawalStatus tracks the approval workflow lifecycle of a wallet withdrawal.
type WithdrawalStatus string

const (
	WithdrawalPending  WithdrawalStatus = "pending"
	WithdrawalApproved WithdrawalStatus = "approved"
	WithdrawalRejected WithdrawalStatus = "rejected"
)

// WalletWithdrawal records a user withdrawal request subject to administrative approval.
type WalletWithdrawal struct {
	ID                  int64            `json:"id"`
	PublicID            string           `json:"public_id"`
	WalletID            int64            `json:"wallet_id"`
	UserID              int64            `json:"user_id"`
	OrganizationID      *int64           `json:"organization_id,omitempty"`
	Amount              money.Amount     `json:"amount"`
	Currency            string           `json:"currency"`
	PayoutMethodType    string           `json:"payout_method_type"`
	DestinationDetails  string           `json:"destination_details"`
	UserPaymentMethodID *int64           `json:"user_payment_method_id,omitempty"`
	UserNotes           string           `json:"user_notes,omitempty"`
	Status              WithdrawalStatus `json:"status"`
	RejectionReason     string           `json:"rejection_reason,omitempty"`
	ReviewedBy          *int64           `json:"reviewed_by,omitempty"`
	ReviewedAt          *time.Time       `json:"reviewed_at,omitempty"`
	TransactionID       *int64           `json:"transaction_id,omitempty"`
	TransferReceiptURL  string           `json:"transfer_receipt_url,omitempty"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
}

// CanCancel reports whether the withdrawal request can still be cancelled by the user.
func (w *WalletWithdrawal) CanCancel() bool {
	return w != nil && w.Status == WithdrawalPending
}

// AdminWalletWithdrawalView represents an enriched withdrawal request record for admin audit and review.
type AdminWalletWithdrawalView struct {
	ID                  int64            `json:"id"`
	PublicID            string           `json:"public_id"`
	WalletID            int64            `json:"wallet_id"`
	UserID              int64            `json:"user_id"`
	UserName            string           `json:"user_name"`
	UserEmail           string           `json:"user_email"`
	UserPhone           string           `json:"user_phone"`
	OrganizationID      *int64           `json:"organization_id,omitempty"`
	OrganizationName    string           `json:"organization_name"`
	OrganizationType    string           `json:"organization_type"`
	Amount              money.Amount     `json:"amount"`
	Currency            string           `json:"currency"`
	PayoutMethodType    string           `json:"payout_method_type"`
	DestinationDetails  string           `json:"destination_details"`
	UserPaymentMethodID *int64           `json:"user_payment_method_id,omitempty"`
	UserNotes           string           `json:"user_notes,omitempty"`
	Status              WithdrawalStatus `json:"status"`
	RejectionReason     string           `json:"rejection_reason,omitempty"`
	ReviewedBy          *int64           `json:"reviewed_by,omitempty"`
	ReviewerName        string           `json:"reviewer_name,omitempty"`
	ReviewedAt          *time.Time       `json:"reviewed_at,omitempty"`
	TransactionID       *int64           `json:"transaction_id,omitempty"`
	TransferReceiptURL  string           `json:"transfer_receipt_url,omitempty"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
}

// WithdrawalFilter specifies parameters for querying wallet withdrawal requests.
type WithdrawalFilter struct {
	UserID           int64
	WalletID         int64
	OrganizationID   int64
	Status           string // "pending", "approved", "rejected", ""
	PayoutMethodType string
	Search           string
	Limit            int
	Offset           int
}
