package pages

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
)

// A6: the admin order badge names the partial states instead of showing the raw key
// in the default grey style.
func TestAdminPaymentBadgeNamesThePartialStates(t *testing.T) {
	cases := []struct {
		status commerce.PaymentStatus
		want   string
	}{
		{commerce.PaymentPartiallyPaid, "مدفوع جزئياً"},
		{commerce.PaymentPartiallyRefunded, "مسترجع جزئياً"},
		{commerce.PaymentRefunded, "مسترجع"},
	}
	for _, tc := range cases {
		label, class := formatAdminPaymentStatusBadge(tc.status)
		if label != tc.want {
			t.Fatalf("%s: label %q, want %q", tc.status, label, tc.want)
		}
		if class == "badge-slate" {
			t.Fatalf("%s: grey default badge", tc.status)
		}
	}
}
