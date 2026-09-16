package postgres

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// textJSON renders a translated name as the jsonb literal the column stores,
// or "" when there is nothing to say.
func textJSON(t i18n.Text) string {
	if t == nil || t.IsEmpty() {
		return ""
	}
	raw, err := json.Marshal(t)
	if err != nil {
		return ""
	}
	return string(raw)
}

// amountText renders a price, or "" when the file stated none. Zero counts as
// none: a supplier row with no price is a row we know nothing about, not a row
// that is free.
func amountText(a money.Amount) string {
	if !a.IsPositive() {
		return ""
	}
	return a.String()
}

// amountTextOrZero renders an amount, keeping an explicit zero.
func amountTextOrZero(a money.Amount) string {
	if a.IsZero() {
		return "0"
	}
	return a.String()
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func dateText(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

func idText(id *int64) string {
	if id == nil || *id <= 0 {
		return ""
	}
	return strconv.FormatInt(*id, 10)
}

// nullableID turns a zero product id into a NULL, which catalog.product_variants
// accepts for an offer bundle that belongs to no catalogue product.
func nullableID(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}

// writeFailureMessage turns a database refusal into something a vendor can act
// on, without leaking constraint names or driver internals — those go to the
// logs through the run's failure record, not to the results screen.
func writeFailureMessage(err error) string {
	if database.IsNotFound(err) {
		return i18n.TDefault("w4_mod.s_351_351")
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "duplicate key"):
		return i18n.TDefault("w4_mod.s_352_352")
	case strings.Contains(msg, "violates check constraint"):
		return i18n.TDefault("w4_mod.s_353_353")
	case strings.Contains(msg, "violates foreign key"):
		return i18n.TDefault("w4_mod.s_354_354")
	case strings.Contains(msg, "numeric field overflow"):
		return i18n.TDefault("w4_mod.s_355_355")
	}
	return i18n.TDefault("w4_mod.w4str_125_125")
}
