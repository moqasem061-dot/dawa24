package pages_test

import (
	"context"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestVendorPaymentsPage_CustomerSearchInputAndDatalist(t *testing.T) {
	data := pages.VendorPaymentsPageData{
		Stats: &billing.VendorPaymentStats{
			TotalCount:  5,
			TotalAmount: money.FromMinor(250000),
		},
		Search:   "INV-999",
		Customer: "صيدلية النور",
		CustomerOrgs: []*billing.CustomerOrgSummary{
			{ID: 101, Name: "صيدلية النور", Code: "PH-101"},
			{ID: 102, Name: "صيدلية الشفاء", Code: "PH-102"},
		},
		Lang: "ar",
		Dir:  "rtl",
	}

	var buf strings.Builder
	err := pages.VendorPaymentsPage(data).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render VendorPaymentsPage: %v", err)
	}
	html := buf.String()

	// Verify customer input box
	if !strings.Contains(html, `name="customer"`) {
		t.Error("expected input box with name='customer'")
	}
	if !strings.Contains(html, `list="vendor-customer-orgs-list"`) {
		t.Error("expected input box with list='vendor-customer-orgs-list'")
	}
	if !strings.Contains(html, `value="صيدلية النور"`) {
		t.Error("expected input box value to contain 'صيدلية النور'")
	}

	// Verify datalist with customer options
	if !strings.Contains(html, `<datalist id="vendor-customer-orgs-list">`) {
		t.Error("expected datalist with id='vendor-customer-orgs-list'")
	}
	if !strings.Contains(html, `value="صيدلية النور"`) || !strings.Contains(html, `value="صيدلية الشفاء"`) {
		t.Error("expected datalist options for 'صيدلية النور' and 'صيدلية الشفاء'")
	}
}

func TestWalletModalWithdraw_PaymentMethodsDropdownAndEmptyAlert(t *testing.T) {
	t.Run("renders select with Arabic label and payment method options", func(t *testing.T) {
		pm := &billing.UserPaymentMethod{
			ID:                42,
			UserID:            10,
			Provider:          "bank",
			AccountIdentifier: "EG990001000200030004",
			Details: billing.PaymentMethodDetails{
				BankName:      "CIB",
				AccountHolder: "صيدلية الأمل",
				IBAN:          "EG990001000200030004",
			},
			IsDefault: true,
		}

		wallet := &billing.Wallet{
			ID:               1,
			Balance:          money.FromMinor(500000),
			AvailableBalance: money.FromMinor(500000),
		}

		data := pages.WalletViewData{
			IsVendor:       true,
			Wallet:         wallet,
			PaymentMethods: []*billing.UserPaymentMethod{pm},
		}

		var buf strings.Builder
		err := pages.WalletModalWithdraw(data, "ar").Render(context.Background(), &buf)
		if err != nil {
			t.Fatalf("failed to render WalletModalWithdraw: %v", err)
		}
		html := buf.String()

		// Verify that "-- common.select --" is NOT rendered, but Arabic "-- اختر من القائمة... --" is!
		if strings.Contains(html, "common.select") {
			t.Errorf("untranslated key 'common.select' found in rendered HTML: %s", html)
		}
		if !strings.Contains(html, "اختر من القائمة...") {
			t.Errorf("expected '-- اختر من القائمة... --' placeholder, got: %s", html)
		}

		// Verify user payment method option
		if !strings.Contains(html, `value="42"`) {
			t.Error("expected option value='42'")
		}
		if !strings.Contains(html, "حساب بنكي (CIB)") || !strings.Contains(html, "EG990001000200030004") {
			t.Errorf("expected option display label with bank name and identifier, got: %s", html)
		}
	})

	t.Run("renders warning alert when no payment methods are registered", func(t *testing.T) {
		wallet := &billing.Wallet{
			ID:               1,
			Balance:          money.FromMinor(500000),
			AvailableBalance: money.FromMinor(500000),
		}

		data := pages.WalletViewData{
			IsVendor:       true,
			Wallet:         wallet,
			PaymentMethods: []*billing.UserPaymentMethod{},
		}

		var buf strings.Builder
		err := pages.WalletModalWithdraw(data, "ar").Render(context.Background(), &buf)
		if err != nil {
			t.Fatalf("failed to render WalletModalWithdraw: %v", err)
		}
		html := buf.String()

		if !strings.Contains(html, "لا توجد وسائل دفع أو حسابات بنكية مسجلة") {
			t.Error("expected alert warning 'لا توجد وسائل دفع أو حسابات بنكية مسجلة'")
		}
		if !strings.Contains(html, "إضافة وسيلة دفع الآن") {
			t.Error("expected button 'إضافة وسيلة دفع الآن'")
		}
	})
}
