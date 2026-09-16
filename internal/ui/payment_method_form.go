package ui

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Reading a payment-method form.
//
// It produces two things from one submission: the rendered line every screen
// displays, and the fields it was composed from. Storing only the line is what
// made editing impossible — there is no way back from
// "CIB • أحمد محمد • IBAN: EG38..." to the three inputs that produced it, so
// the edit route existed for months with no screen able to offer a form.
//
// The provider itself arrives under either name. The wallet modal posts
// "provider" and this function only ever read "type", so every add from
// /customer/wallet and /vendor/wallet fell through to the default arm and
// answered "نوع وسيلة الدفع غير صالح" — the form was never invalid, it was
// never read.

// paymentMethodInput is one validated payment-method submission.
type paymentMethodInput struct {
	Provider   string
	Identifier string
	Details    billing.PaymentMethodDetails
}

// readPaymentMethodForm parses and validates a payment-method form.
func readPaymentMethodForm(r *http.Request) (*paymentMethodInput, error) {
	lang := langOf(r)

	payType := strings.TrimSpace(r.PostFormValue("type"))
	if payType == "" {
		payType = strings.TrimSpace(r.PostFormValue("provider"))
	}

	switch normalizePaymentProvider(payType) {
	case "bank":
		return readBankMethod(r, lang)
	case "instapay":
		return readInstapayMethod(r, lang)
	case "wallet":
		return readWalletMethod(r, lang)
	case "card":
		return readCardMethod(r, lang)
	default:
		return nil, fmt.Errorf("%s", i18n.T(lang, "payment.invalid_type"))
	}
}

// normalizePaymentProvider folds the form's vocabulary onto the four values the
// column stores.
//
// The wallet modal offers "vodafone_cash" and the database holds "wallet"; the
// two have to agree or a saved method reopens with no provider selected.
func normalizePaymentProvider(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "bank", "bank_transfer":
		return "bank"
	case "instapay":
		return "instapay"
	case "wallet", "vodafone_cash", "e_wallet", "mobile_wallet":
		return "wallet"
	case "card", "credit_card", "debit_card":
		return "card"
	default:
		return ""
	}
}

func readBankMethod(r *http.Request, lang string) (*paymentMethodInput, error) {
	d := billing.PaymentMethodDetails{
		BankName:      strings.TrimSpace(r.PostFormValue("bank_name")),
		AccountHolder: strings.TrimSpace(r.PostFormValue("account_holder")),
		IBAN:          strings.TrimSpace(r.PostFormValue("iban")),
		AccountNumber: strings.TrimSpace(r.PostFormValue("account_number")),
		SwiftCode:     strings.TrimSpace(r.PostFormValue("swift_code")),
		BranchName:    strings.TrimSpace(r.PostFormValue("branch_name")),
	}
	if d.IBAN == "" && d.AccountNumber == "" {
		return nil, fmt.Errorf("%s", i18n.T(lang, "payment.iban_or_account_required"))
	}
	if d.BankName == "" {
		d.BankName = i18n.T(lang, "payment.bank_account")
	}

	parts := []string{d.BankName}
	if d.AccountHolder != "" {
		parts = append(parts, d.AccountHolder)
	}
	if d.IBAN != "" {
		parts = append(parts, "IBAN: "+d.IBAN)
	}
	if d.AccountNumber != "" {
		parts = append(parts, i18n.T(lang, "payment.account_prefix")+d.AccountNumber)
	}
	if d.SwiftCode != "" {
		parts = append(parts, "SWIFT: "+d.SwiftCode)
	}
	if d.BranchName != "" {
		parts = append(parts, i18n.T(lang, "payment.branch_prefix")+d.BranchName)
	}
	return &paymentMethodInput{Provider: "bank", Identifier: strings.Join(parts, " • "), Details: d}, nil
}

func readInstapayMethod(r *http.Request, lang string) (*paymentMethodInput, error) {
	d := billing.PaymentMethodDetails{
		InstapayHandle: strings.TrimSpace(r.PostFormValue("instapay_handle")),
		AccountHolder:  strings.TrimSpace(r.PostFormValue("account_holder")),
	}
	if d.InstapayHandle == "" {
		return nil, fmt.Errorf("%s", i18n.T(lang, "payment.instapay_required"))
	}
	identifier := "InstaPay: " + d.InstapayHandle
	if d.AccountHolder != "" {
		identifier = fmt.Sprintf("InstaPay: %s • %s", d.InstapayHandle, d.AccountHolder)
	}
	return &paymentMethodInput{Provider: "instapay", Identifier: identifier, Details: d}, nil
}

func readWalletMethod(r *http.Request, lang string) (*paymentMethodInput, error) {
	d := billing.PaymentMethodDetails{
		WalletProvider: strings.TrimSpace(r.PostFormValue("wallet_provider")),
		WalletPhone:    strings.TrimSpace(r.PostFormValue("wallet_phone")),
		AccountHolder:  strings.TrimSpace(r.PostFormValue("account_holder")),
	}
	if d.WalletPhone == "" {
		return nil, fmt.Errorf("%s", i18n.T(lang, "payment.wallet_phone_required"))
	}
	if d.WalletProvider == "" {
		d.WalletProvider = i18n.T(lang, "payment.e_wallet")
	}
	identifier := fmt.Sprintf("%s: %s", d.WalletProvider, d.WalletPhone)
	if d.AccountHolder != "" {
		identifier = fmt.Sprintf("%s: %s • %s", d.WalletProvider, d.WalletPhone, d.AccountHolder)
	}
	return &paymentMethodInput{Provider: "wallet", Identifier: identifier, Details: d}, nil
}

func readCardMethod(r *http.Request, lang string) (*paymentMethodInput, error) {
	number := strings.ReplaceAll(strings.TrimSpace(r.PostFormValue("card_number")), " ", "")
	if number == "" {
		return nil, fmt.Errorf("%s", i18n.T(lang, "payment.card_number_required"))
	}

	// Only the last four digits are ever kept. The full number is not stored
	// here, is not stored in details, and is not logged.
	last4 := number
	if len(last4) > 4 {
		last4 = last4[len(last4)-4:]
	}
	d := billing.PaymentMethodDetails{
		CardBrand: strings.TrimSpace(r.PostFormValue("card_brand")),
		CardName:  strings.TrimSpace(r.PostFormValue("card_name")),
		CardLast4: last4,
	}
	if d.CardBrand == "" {
		d.CardBrand = "Card"
	}
	identifier := fmt.Sprintf("%s (•••• %s)", d.CardBrand, d.CardLast4)
	if d.CardName != "" {
		identifier = fmt.Sprintf("%s (•••• %s) - %s", d.CardBrand, d.CardLast4, d.CardName)
	}
	return &paymentMethodInput{Provider: "card", Identifier: identifier, Details: d}, nil
}

// paymentMethodFormValue is the option the edit dialog should preselect.
//
// The stored provider is "wallet" while the dialog's option is
// "vodafone_cash", so reopening a saved wallet without this mapping showed the
// first option — a bank account — over a wallet's fields.
func paymentMethodFormValue(stored string) string {
	if normalizePaymentProvider(stored) == "wallet" {
		return "vodafone_cash"
	}
	return normalizePaymentProvider(stored)
}
