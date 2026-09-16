package pages

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
)

type WalletViewData struct {
	IsVendor               bool
	Wallet                 *billing.Wallet
	Transactions           []*billing.WalletTransaction
	DepositRequests        []*billing.WalletDeposit
	WithdrawalRequests     []*billing.WalletWithdrawal
	PaymentMethods         []*billing.UserPaymentMethod
	PlatformPaymentMethods []*billing.PlatformPaymentMethod
	NoticeType             string
	NoticeMessage          string
	// TxStatus filters billing.wallet_deposits.status: pending|completed|rejected.
	TxStatus string
	// TxType filters billing.wallet_transactions.type: deposit|withdrawal|purchase.
	TxType     string
	Page       int
	PerPage    int
	TotalCount int
}

func walletBaseURL(isVendor bool) string {
	if isVendor {
		return "/vendor/wallet"
	}
	return "/customer/wallet"
}

func walletTxQueryValues(data WalletViewData) url.Values {
	q := url.Values{}
	if data.TxStatus != "" {
		q.Set("status", data.TxStatus)
	}
	if data.TxType != "" {
		q.Set("type", data.TxType)
	}
	return q
}

func walletBalance(w *billing.Wallet) string {
	if w == nil {
		return "0.00 ج.م"
	}
	return w.Balance.String() + " ج.م"
}

func walletBalanceClass(w *billing.Wallet) string {
	if w == nil || w.Balance.Minor() == 0 {
		return "text-primary"
	}
	if w.Balance.Minor() < 0 {
		return "text-rose text-danger"
	}
	return "text-emerald text-success"
}

func walletAvailableBalance(w *billing.Wallet) string {
	if w == nil {
		return "0.00 ج.م"
	}
	return w.AvailableBalance.String() + " ج.م"
}

func walletAvailableBalanceClass(w *billing.Wallet) string {
	if w == nil || w.AvailableBalance.Minor() == 0 {
		return "text-primary"
	}
	if w.AvailableBalance.Minor() < 0 {
		return "text-rose text-danger"
	}
	return "text-emerald text-success"
}

func walletPendingWithdrawal(w *billing.Wallet) string {
	if w == nil {
		return "0.00 ج.م"
	}
	return w.PendingWithdrawal.String() + " ج.م"
}

func computePendingDepositsCount(deps []*billing.WalletDeposit) int {
	count := 0
	for _, d := range deps {
		if d != nil && d.Status != "approved" {
			count++
		}
	}
	return count
}

func computePendingWithdrawalsCount(withs []*billing.WalletWithdrawal) int {
	count := 0
	for _, w := range withs {
		if w != nil && w.Status == billing.WithdrawalPending {
			count++
		}
	}
	return count
}

func txTypeBadgeClass(t billing.TransactionType) string {
	switch t {
	case billing.TxDeposit:
		return "badge-emerald"
	case billing.TxWithdrawal:
		return "badge-amber"
	case billing.TxPurchase:
		return "badge-purple"
	case billing.TxRefund:
		return "badge-cyan"
	case billing.TxBonus:
		return "badge-indigo"
	case billing.TxTransferIn:
		return "badge-teal"
	case billing.TxTransferOut:
		return "badge-orange"
	case billing.TxPenalty:
		return "badge-pink"
	default:
		return "badge-gray"
	}
}

func txTypeLabel(t billing.TransactionType) string {
	switch t {
	case billing.TxDeposit:
		return "إيداع رصيد"
	case billing.TxWithdrawal:
		return "سحب رصيد"
	case billing.TxPurchase:
		return "سداد فاتورة/طلب"
	case billing.TxRefund:
		return "استرداد أموال"
	case billing.TxBonus:
		return "مكافأة / بونص"
	case billing.TxTransferIn:
		return "تحويل وارد"
	case billing.TxTransferOut:
		return "تحويل صادر"
	case billing.TxPenalty:
		return "غرامة/خصم"
	default:
		return "معاملة مالية"
	}
}

func paymentProviderTitle(provider string) string {
	switch strings.ToLower(provider) {
	case "instapay":
		return "إنستاباي (InstaPay)"
	case "vodafone_cash", "wallet":
		return "محفظة هاتف ذكي"
	case "card":
		return "بطاقة بنكية"
	case "bank", "bank_transfer":
		return "تحويل بنكي رسمي"
	default:
		return provider
	}
}

type platformMethodClientItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ProviderType  string `json:"provider_type"`
	AccountName   string `json:"account_name"`
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
	IBAN          string `json:"iban"`
	InstaPay      string `json:"instapay_handle"`
	PhoneNumber   string `json:"phone_number"`
	Description   string `json:"description"`
}

func platformMethodsJSON(ppms []*billing.PlatformPaymentMethod) string {
	var list []platformMethodClientItem
	for _, p := range ppms {
		if p == nil || !p.IsActive || !p.IsDepositEnabled {
			continue
		}
		list = append(list, platformMethodClientItem{
			ID:            p.ID,
			Name:          p.Name.Get("ar"),
			ProviderType:  p.ProviderType,
			AccountName:   p.AccountName,
			BankName:      p.BankName,
			AccountNumber: p.AccountNumber,
			IBAN:          p.IBAN,
			InstaPay:      p.InstaPayHandle,
			PhoneNumber:   p.PhoneNumber,
			Description:   p.Description.Get("ar"),
		})
	}
	b, err := json.Marshal(list)
	if err != nil {
		return "[]"
	}
	return string(b)
}

type userMethodClientItem struct {
	ID           int64  `json:"id"`
	Provider     string `json:"provider"`
	Identifier   string `json:"identifier"`
	IsDefault    bool   `json:"is_default"`
	Details      string `json:"details"`
	DisplayLabel string `json:"display_label"`
}

func userMethodDisplayLabel(m *billing.UserPaymentMethod) string {
	if m == nil {
		return ""
	}
	providerName := ""
	switch strings.ToLower(m.Provider) {
	case "bank":
		if m.Details.BankName != "" {
			providerName = "حساب بنكي (" + m.Details.BankName + ")"
		} else {
			providerName = "حساب بنكي"
		}
	case "instapay":
		providerName = "إنستاباي (InstaPay)"
	case "wallet", "vodafone_cash":
		if m.Details.WalletProvider != "" {
			providerName = "محفظة إلكترونية (" + m.Details.WalletProvider + ")"
		} else {
			providerName = "محفظة إلكترونية"
		}
	case "card":
		providerName = "بطاقة بنكية"
	default:
		providerName = m.Provider
	}

	label := providerName + " - " + m.AccountIdentifier
	if m.Details.AccountHolder != "" {
		label += " (" + m.Details.AccountHolder + ")"
	}
	if m.IsDefault {
		label += " ★ الافتراضي"
	}
	return label
}

func userPaymentMethodsJSON(pms []*billing.UserPaymentMethod) string {
	var list []userMethodClientItem
	for _, m := range pms {
		if m == nil {
			continue
		}
		det := m.Details.AccountHolder
		if m.Details.IBAN != "" {
			det += " • " + m.Details.IBAN
		} else if m.Details.InstapayHandle != "" {
			det += " • " + m.Details.InstapayHandle
		} else if m.Details.WalletPhone != "" {
			det += " • " + m.Details.WalletPhone
		}
		list = append(list, userMethodClientItem{
			ID:           m.ID,
			Provider:     m.Provider,
			Identifier:   m.AccountIdentifier,
			IsDefault:    m.IsDefault,
			Details:      strings.TrimSpace(det),
			DisplayLabel: userMethodDisplayLabel(m),
		})
	}
	b, err := json.Marshal(list)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// walletState outputs the Alpine x-data configuration for the wallet page.
// It wires platform supported deposit channels with matching user payment methods,
// and supports the withdrawal flow with live balance checks.
func walletState(data WalletViewData) string {
	base := walletBaseURL(data.IsVendor)
	pmsJSON := platformMethodsJSON(data.PlatformPaymentMethods)
	umsJSON := userPaymentMethodsJSON(data.PaymentMethods)

	return fmt.Sprintf(`dawaWallet({
		base: '%s',
		platformMethods: %s,
		userMethods: %s
	})`, base, pmsJSON, umsJSON)
}

func paymentMethodJSON(pm *billing.UserPaymentMethod) string {
	if pm == nil {
		return "{}"
	}
	payload := struct {
		ID             int64  `json:"id"`
		Type           string `json:"type"`
		IsDefault      bool   `json:"is_default"`
		AccountHolder  string `json:"account_holder"`
		BankName       string `json:"bank_name"`
		IBAN           string `json:"iban"`
		AccountNumber  string `json:"account_number"`
		InstapayHandle string `json:"instapay_handle"`
		WalletProvider string `json:"wallet_provider"`
		WalletPhone    string `json:"wallet_phone"`
		CardBrand      string `json:"card_brand"`
		CardLast4      string `json:"card_last4"`
	}{
		ID:             pm.ID,
		Type:           PaymentMethodFormValue(pm.Provider),
		IsDefault:      pm.IsDefault,
		AccountHolder:  pm.Details.AccountHolder,
		BankName:       pm.Details.BankName,
		IBAN:           pm.Details.IBAN,
		AccountNumber:  pm.Details.AccountNumber,
		InstapayHandle: pm.Details.InstapayHandle,
		WalletProvider: pm.Details.WalletProvider,
		WalletPhone:    pm.Details.WalletPhone,
		CardBrand:      pm.Details.CardBrand,
		CardLast4:      pm.Details.CardLast4,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func PaymentMethodFormValue(stored string) string {
	switch stored {
	case "wallet":
		return "vodafone_cash"
	case "bank", "instapay", "card":
		return stored
	default:
		return "bank"
	}
}
