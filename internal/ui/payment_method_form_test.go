package ui

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func paymentForm(values url.Values) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/customer/wallet/payment-methods",
		strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_ = r.ParseForm()
	return r
}

// The wallet dialog posts "provider"; the handler read "type".
//
// This is the whole of the reported bug: every add from /customer/wallet and
// /vendor/wallet answered "نوع وسيلة الدفع غير صالح" because the field the form
// sent was never read, so the switch always reached its default arm. Both names
// are accepted, permanently — three handlers share this function and a page
// cached in a browser may still post either.
func TestPaymentFormAcceptsBothFieldNames(t *testing.T) {
	for _, key := range []string{"type", "provider"} {
		t.Run(key, func(t *testing.T) {
			in, err := readPaymentMethodForm(paymentForm(url.Values{
				key:         {"bank"},
				"iban":      {"EG380003000123456789"},
				"bank_name": {"CIB"},
			}))
			if err != nil {
				t.Fatalf("readPaymentMethodForm: %v", err)
			}
			if in.Provider != "bank" {
				t.Errorf("provider = %q, want bank", in.Provider)
			}
		})
	}
}

// A saved method has to be reopenable, which means the fields survive.
func TestPaymentFormKeepsStructuredFields(t *testing.T) {
	in, err := readPaymentMethodForm(paymentForm(url.Values{
		"type":           {"bank"},
		"bank_name":      {"CIB"},
		"account_holder": {"أحمد محمد"},
		"iban":           {"EG380003000123456789"},
	}))
	if err != nil {
		t.Fatalf("readPaymentMethodForm: %v", err)
	}
	if in.Details.BankName != "CIB" || in.Details.IBAN != "EG380003000123456789" || in.Details.AccountHolder != "أحمد محمد" {
		t.Errorf("structured details lost: %+v", in.Details)
	}
	// And the rendered line every screen displays is still produced.
	for _, want := range []string{"CIB", "أحمد محمد", "IBAN: EG380003000123456789"} {
		if !strings.Contains(in.Identifier, want) {
			t.Errorf("rendered identifier %q is missing %q", in.Identifier, want)
		}
	}
}

// The dialog's vocabulary and the column's have to agree.
func TestPaymentProviderNormalisation(t *testing.T) {
	for form, stored := range map[string]string{
		"bank": "bank", "bank_transfer": "bank",
		"instapay":      "instapay",
		"vodafone_cash": "wallet", "wallet": "wallet", "e_wallet": "wallet",
		"card": "card", "credit_card": "card",
		"bitcoin": "", "": "",
	} {
		if got := normalizePaymentProvider(form); got != stored {
			t.Errorf("normalizePaymentProvider(%q) = %q, want %q", form, got, stored)
		}
	}

	// And back, for the edit dialog: the column holds "wallet" while the
	// dialog's option is "vodafone_cash". Without the return trip a saved
	// wallet reopened showing a bank account's fields.
	if got := paymentMethodFormValue("wallet"); got != "vodafone_cash" {
		t.Errorf("paymentMethodFormValue(wallet) = %q, want vodafone_cash", got)
	}
	if got := paymentMethodFormValue("bank"); got != "bank" {
		t.Errorf("paymentMethodFormValue(bank) = %q, want bank", got)
	}
}

// A card's number is never stored — only its last four digits.
func TestPaymentFormStoresOnlyCardLast4(t *testing.T) {
	full := "4111222233334444"
	in, err := readPaymentMethodForm(paymentForm(url.Values{
		"type":        {"card"},
		"card_number": {"4111 2222 3333 4444"},
		"card_brand":  {"Visa"},
	}))
	if err != nil {
		t.Fatalf("readPaymentMethodForm: %v", err)
	}
	if in.Details.CardLast4 != "4444" {
		t.Errorf("card_last4 = %q, want 4444", in.Details.CardLast4)
	}
	if strings.Contains(in.Identifier, full) {
		t.Error("the full card number reached the rendered identifier")
	}
	if strings.Contains(in.Details.CardBrand+in.Details.CardName+in.Details.CardLast4, full) {
		t.Error("the full card number reached the stored details")
	}
}

// Each provider's own required field is enforced.
func TestPaymentFormRequiresProviderFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		form url.Values
	}{
		{"unknown provider", url.Values{"type": {"bitcoin"}}},
		{"no provider at all", url.Values{}},
		{"bank without iban or account", url.Values{"type": {"bank"}, "bank_name": {"CIB"}}},
		{"instapay without a handle", url.Values{"type": {"instapay"}}},
		{"wallet without a phone", url.Values{"type": {"vodafone_cash"}}},
		{"card without a number", url.Values{"type": {"card"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := readPaymentMethodForm(paymentForm(tc.form)); err == nil {
				t.Error("expected a validation error")
			}
		})
	}
}
