package pages

import (
	"encoding/json"
	"net/url"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type customerOrgItemJSON struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

func customerOrgsToJSON(orgs []*billing.CustomerOrgSummary) string {
	items := make([]customerOrgItemJSON, 0, len(orgs))
	for _, o := range orgs {
		if o != nil && o.Name != "" {
			items = append(items, customerOrgItemJSON{
				ID:   o.ID,
				Name: o.Name,
				Code: o.Code,
			})
		}
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// VendorPaymentsPageData carries all view state for the vendor payments ledger.
type VendorPaymentsPageData struct {
	Payments     []*billing.AdminPaymentView
	Invoices     []*billing.AdminInvoiceView
	Stats        *billing.VendorPaymentStats
	Search       string
	Customer     string
	CustomerOrgs []*billing.CustomerOrgSummary
	Method       string
	Status       string
	DateFrom     string
	DateTo       string
	Page         int
	PerPage      int
	TotalCount   int
	Lang         string
	Dir          string
}

func vendorPaymentMethodLabel(m string, lang string) string {
	switch m {
	case "bank_transfer":
		return i18n.T(lang, "vendor_finance.method.bank_transfer")
	case "cash":
		return i18n.T(lang, "vendor_finance.method.cash")
	case "cheque":
		return i18n.T(lang, "vendor_finance.method.cheque")
	case "electronic_wallet":
		return i18n.T(lang, "vendor_finance.method.electronic_wallet")
	default:
		if m == "" {
			return i18n.T(lang, "vendor_finance.method.unspecified")
		}
		return m
	}
}

func vendorPaymentStatusBadge(s string, lang string) (string, string) {
	switch s {
	case "paid", "completed":
		return i18n.T(lang, "vendor_finance.status.paid"), "badge-emerald"
	case "pending":
		return i18n.T(lang, "vendor_finance.status.pending"), "badge-amber"
	case "failed":
		return i18n.T(lang, "vendor_finance.status.failed"), "badge-rose"
	case "refunded":
		return i18n.T(lang, "vendor_finance.status.refunded"), "badge-slate"
	default:
		return s, "badge-slate"
	}
}

func vendorPaymentsQueryValues(data VendorPaymentsPageData) url.Values {
	q := url.Values{}
	if data.Search != "" {
		q.Set("q", data.Search)
	}
	if data.Customer != "" {
		q.Set("customer", data.Customer)
	}
	if data.Method != "" {
		q.Set("method", data.Method)
	}
	if data.Status != "" {
		q.Set("status", data.Status)
	}
	if data.DateFrom != "" {
		q.Set("from", data.DateFrom)
	}
	if data.DateTo != "" {
		q.Set("to", data.DateTo)
	}
	return q
}
