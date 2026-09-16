package billing

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// A saved payment method, and the structured fields behind its rendered line.
//
// Split from domain.go for the 400-line rule.

// UserPaymentMethod represents a stored payment method identifier.
type UserPaymentMethod struct {
	ID       int64  `json:"id"`
	PublicID string `json:"public_id"`
	UserID   int64  `json:"user_id"`
	// Provider is one of bank, instapay, wallet, card.
	Provider string `json:"provider"`
	// AccountIdentifier is the rendered line every screen displays:
	// "CIB • أحمد محمد • IBAN: EG38...". It is built for reading, not parsing.
	AccountIdentifier string `json:"account_identifier"`
	// Details holds the same information as the fields it was composed from.
	//
	// Without it a saved method could be displayed and deleted but never
	// edited: there is no way back from the rendered line to the bank name,
	// the holder and the IBAN. The edit route existed for months with no screen
	// able to offer a form for it.
	Details   PaymentMethodDetails `json:"details"`
	IsDefault bool                 `json:"is_default"`
	CreatedAt time.Time            `json:"created_at"`
}

// PaymentMethodDetails is the submitted shape of one payment method.
//
// One struct covering all four providers rather than four types: the fields do
// not overlap, and a saved row has to be reopened by a form that does not yet
// know which provider it is about.
type PaymentMethodDetails struct {
	BankName      string `json:"bank_name,omitempty"`
	AccountHolder string `json:"account_holder,omitempty"`
	IBAN          string `json:"iban,omitempty"`
	AccountNumber string `json:"account_number,omitempty"`
	SwiftCode     string `json:"swift_code,omitempty"`
	BranchName    string `json:"branch_name,omitempty"`

	InstapayHandle string `json:"instapay_handle,omitempty"`

	WalletProvider string `json:"wallet_provider,omitempty"`
	WalletPhone    string `json:"wallet_phone,omitempty"`

	// CardLast4 and CardBrand are all that is kept of a card. The full number
	// is never stored, and never was.
	CardBrand string `json:"card_brand,omitempty"`
	CardLast4 string `json:"card_last4,omitempty"`
	CardName  string `json:"card_name,omitempty"`
}

// Value implements driver.Valuer for the JSONB details column.
func (d PaymentMethodDetails) Value() (driver.Value, error) {
	return json.Marshal(d)
}

// Scan implements sql.Scanner for the JSONB details column.
//
// A NULL or an empty payload is the zero value rather than an error: every row
// written before the column existed has one, and a saved method that predates
// it must still list and delete normally — it simply opens its edit form with
// the sub-fields blank.
func (d *PaymentMethodDetails) Scan(src any) error {
	if src == nil {
		*d = PaymentMethodDetails{}
		return nil
	}
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("billing: cannot scan %T into PaymentMethodDetails", src)
	}
	if len(raw) == 0 || string(raw) == "null" {
		*d = PaymentMethodDetails{}
		return nil
	}
	return json.Unmarshal(raw, d)
}
