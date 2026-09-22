package pages

import (
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
)

func TestFormatPaymentMethodName(t *testing.T) {
	cases := []struct {
		method   string
		lang     string
		expected string
	}{
		{"cod", "ar", "الدفع عند الاستلام"},
		{"cod", "en", "Cash on Delivery"},
		{"wallet", "ar", "المحفظة الإلكترونية"},
		{"wallet", "en", "E-Wallet"},
		{"instapay", "ar", "إنستاباي (InstaPay)"},
		{"instapay", "en", "InstaPay"},
		{"card", "ar", "بطاقة بنكية"},
		{"bank_transfer", "ar", "تحويل بنكي"},
		{"fawry", "ar", "فوري (Fawry)"},
		{"vodafone_cash", "ar", "فودافون كاش"},
		{"", "ar", "الدفع عند الاستلام"},
	}

	for _, tc := range cases {
		got := FormatPaymentMethodName(tc.method, tc.lang)
		if got != tc.expected {
			t.Errorf("FormatPaymentMethodName(%q, %q) = %q; want %q", tc.method, tc.lang, got, tc.expected)
		}
	}
}

func TestIsPrepaidPaymentMethod(t *testing.T) {
	if IsPrepaidPaymentMethod("cod") {
		t.Error("expected cod to not be prepaid")
	}
	if IsPrepaidPaymentMethod("") {
		t.Error("expected empty to not be prepaid")
	}
	if IsPrepaidPaymentMethod("cash") {
		t.Error("expected cash to not be prepaid")
	}
	if !IsPrepaidPaymentMethod("instapay") {
		t.Error("expected instapay to be prepaid")
	}
	if !IsPrepaidPaymentMethod("wallet") {
		t.Error("expected wallet to be prepaid")
	}
	if !IsPrepaidPaymentMethod("card") {
		t.Error("expected card to be prepaid")
	}
}

func TestFormatPaymentStatusLabel(t *testing.T) {
	// Paid
	if got := FormatPaymentStatusLabel(commerce.PaymentPaid, "instapay", "ar"); got != "مدفوع بالكامل" {
		t.Errorf("got %q, want 'مدفوع بالكامل'", got)
	}
	// Refunded
	if got := FormatPaymentStatusLabel(commerce.PaymentRefunded, "wallet", "ar"); got != "مسترجع" {
		t.Errorf("got %q, want 'مسترجع'", got)
	}
	// COD unpaid without order status
	if got := FormatPaymentStatusLabel(commerce.PaymentUnpaid, "cod", "ar"); got != "مستحق عند الاستلام" {
		t.Errorf("got %q, want 'مستحق عند الاستلام'", got)
	}
	// COD unpaid with pending order status
	if got := FormatPaymentStatusLabel(commerce.PaymentUnpaid, "cod", "ar", commerce.StatusPending); got != "مستحق عند الاستلام (بانتظار تأكيد المورد)" {
		t.Errorf("got %q, want 'مستحق عند الاستلام (بانتظار تأكيد المورد)'", got)
	}
	// COD unpaid with confirmed order status
	if got := FormatPaymentStatusLabel(commerce.PaymentUnpaid, "cod", "ar", commerce.StatusConfirmed); got != "مؤكد (مستحق عند الاستلام)" {
		t.Errorf("got %q, want 'مؤكد (مستحق عند الاستلام)'", got)
	}
	// COD unpaid badge class with confirmed order status
	if got := PaymentStatusBadgeClass(commerce.PaymentUnpaid, "cod", commerce.StatusConfirmed); got != "badge-sky" {
		t.Errorf("got %q, want 'badge-sky'", got)
	}
	// InstaPay unpaid
	if got := FormatPaymentStatusLabel(commerce.PaymentUnpaid, "instapay", "ar"); got != "بانتظار تأكيد الدفع الإلكتروني" {
		t.Errorf("got %q, want 'بانتظار تأكيد الدفع الإلكتروني'", got)
	}
	// Wallet authorized
	if got := FormatPaymentStatusLabel(commerce.PaymentAuthorized, "wallet", "ar"); got != "معتمد من المحفظة (يُخصم عند تأكيد المورد)" {
		t.Errorf("got %q, want 'معتمد من المحفظة (يُخصم عند تأكيد المورد)'", got)
	}
}

func TestFormatParcelPINInstruction(t *testing.T) {
	vendor := "شركة الأمل للتوزيع"

	// 1. Prepaid (CollectNothing)
	prepaidInst := FormatParcelPINInstruction(vendor, "instapay", commerce.CollectNothing, "ar")
	if !strings.Contains(prepaidInst, "تم السداد إلكترونياً ولا يُطلب منك أي مبالغ نقدية") {
		t.Errorf("prepaid instruction should indicate no cash required, got %q", prepaidInst)
	}
	if strings.Contains(prepaidInst, "وسداد القيمة") {
		t.Errorf("prepaid instruction should NOT say 'وسداد القيمة', got %q", prepaidInst)
	}

	// 2. COD (CollectFull)
	codInst := FormatParcelPINInstruction(vendor, "cod", commerce.CollectFull, "ar")
	if !strings.Contains(codInst, "وسداد القيمة") {
		t.Errorf("COD instruction should mention paying the amount, got %q", codInst)
	}

	// 3. Shipping only (CollectShippingOnly)
	shipInst := FormatParcelPINInstruction(vendor, "wallet", commerce.CollectShippingOnly, "ar")
	if !strings.Contains(shipInst, "وسداد مصاريف الشحن فقط") {
		t.Errorf("shipping-only instruction should mention shipping only, got %q", shipInst)
	}
}
