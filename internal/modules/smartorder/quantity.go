package smartorder

import (
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"strconv"
	"strings"
)

// Quantity parsing.
//
// A pharmacy's spreadsheet does not contain numbers. It contains "2", " 3 ",
// i18n.TDefault("w4_ui.s_44_44"), i18n.TDefault("w4_mod.5_434"), "2-3", "12.0", "-1", and empty cells that mean "I did not
// decide yet". Every one of those has to become either a quantity or an honest
// admission that it could not be read.
//
// The rule throughout: **never guess silently.** A cell reading "2-3" could
// plausibly become 2, or 3, or 2.5, and any of those choices sends stock to a
// pharmacy that did not order it. The parser returns no quantity and a note
// saying why, the line falls back to the default quantity, and the buyer sees
// what happened in the results table. A wrong order costs more than an
// unanswered cell.

// QtyResult is the outcome of reading one quantity cell.
type QtyResult struct {
	// Qty is nil when the cell could not be read as a single quantity. That is
	// not an error — it means the default quantity applies and the buyer is told.
	Qty  *float64
	Note string
}

// arabicIndicDigits maps Arabic-Indic and Eastern Arabic-Indic numerals onto
// ASCII. Egyptian systems export both, sometimes in the same column.
var arabicIndicDigits = strings.NewReplacer(
	i18n.TDefault("w4_ui.s_39_39"), "0", i18n.TDefault("w4_ui.s_40_40"), "1", i18n.TDefault("w4_ui.s_41_41"), "2", i18n.TDefault("w4_ui.s_42_42"), "3", i18n.TDefault("w4_ui.s_43_43"), "4",
	i18n.TDefault("w4_ui.s_44_44"), "5", i18n.TDefault("w4_ui.s_45_45"), "6", i18n.TDefault("w4_ui.s_46_46"), "7", i18n.TDefault("w4_ui.s_47_47"), "8", i18n.TDefault("w4_ui.s_48_48"), "9",
	i18n.TDefault("w4_mod.s_435_435"), "0", i18n.TDefault("w4_mod.s_436_436"), "1", i18n.TDefault("w4_mod.s_437_437"), "2", i18n.TDefault("w4_mod.s_438_438"), "3", i18n.TDefault("w4_mod.s_439_439"), "4",
	i18n.TDefault("w4_mod.s_440_440"), "5", i18n.TDefault("w4_mod.s_441_441"), "6", i18n.TDefault("w4_mod.s_442_442"), "7", i18n.TDefault("w4_mod.s_443_443"), "8", i18n.TDefault("w4_mod.s_444_444"), "9",
	i18n.TDefault("w4_ui.s_49_49"), ".", // Arabic decimal separator
	i18n.TDefault("w4_mod.s_445_445"), "", // Arabic comma used as a thousands separator
)

// ParseQuantity reads a spreadsheet cell as a requested quantity.
func ParseQuantity(raw string) QtyResult {
	s := strings.TrimSpace(raw)
	if s == "" {
		return QtyResult{} // blank: the default applies, and that is not noteworthy
	}

	s = arabicIndicDigits.Replace(s)
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))

	// A plain number is the overwhelmingly common case.
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return validateQty(v, raw)
	}

	// A range: "2-3", "2 - 3", i18n.TDefault("w4_mod.s_446_446"). Ambiguous by construction.
	if isRange(s) {
		return QtyResult{Note: fmt.Sprintf("cell %q is a range; enter a single quantity", raw)}
	}

	// A number with a unit: i18n.TDefault("w4_mod.5_434"), "5 boxes", "5pcs". The number is usable,
	// but the unit may not be the unit the supplier sells in, so it is recorded.
	if v, unit, ok := numberWithUnit(s); ok {
		res := validateQty(v, raw)
		if res.Qty != nil {
			res.Note = fmt.Sprintf("read %g from %q; unit %q ignored — check it matches the supplier's unit", v, raw, unit)
		}
		return res
	}

	return QtyResult{Note: fmt.Sprintf("could not read a quantity from %q", raw)}
}

// validateQty rejects values that are numbers but not quantities.
func validateQty(v float64, raw string) QtyResult {
	if v < 0 {
		return QtyResult{Note: fmt.Sprintf("cell %q is negative", raw)}
	}
	if v != float64(int64(v)) {
		// Fractional quantities are meaningful for some units and meaningless
		// for boxes. Kept, but flagged, because rounding it here would be the
		// silent guess this parser exists to avoid.
		return QtyResult{Qty: &v, Note: fmt.Sprintf("cell %q is fractional", raw)}
	}
	return QtyResult{Qty: &v}
}

// isRange reports whether the cell looks like "2-3" rather than a negative
// number or a hyphenated code.
func isRange(s string) bool {
	for _, sep := range []string{"-", "–", "—", "to", i18n.TDefault("w4_mod.s_447_447"), i18n.TDefault("w4_mod.s_448_448")} {
		idx := strings.Index(s, sep)
		if idx <= 0 || idx+len(sep) >= len(s) {
			continue
		}
		left := strings.TrimSpace(s[:idx])
		right := strings.TrimSpace(s[idx+len(sep):])
		if _, err := strconv.ParseFloat(left, 64); err != nil {
			continue
		}
		if _, err := strconv.ParseFloat(right, 64); err != nil {
			continue
		}
		return true
	}
	return false
}

// numberWithUnit splits a leading number from a trailing unit word.
func numberWithUnit(s string) (float64, string, bool) {
	end := 0
	for i, r := range s {
		if (r >= '0' && r <= '9') || r == '.' {
			end = i + 1
			continue
		}
		if i == 0 {
			return 0, "", false
		}
		break
	}
	if end == 0 {
		return 0, "", false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s[:end]), 64)
	if err != nil {
		return 0, "", false
	}
	unit := strings.TrimSpace(s[end:])
	if unit == "" {
		return 0, "", false
	}
	return v, unit, true
}

// ApplyQuantities sets each line's effective quantity from the config, and marks
// zero-quantity lines so they are visibly excluded rather than quietly dropped.
func ApplyQuantities(cfg *Config, lines []*Line) {
	for _, l := range lines {
		l.EffectiveQty = cfg.EffectiveQty(l)
		if l.EffectiveQty <= 0 && l.Outcome != OutcomeRemoved {
			l.Outcome = OutcomeZeroQty
		}
	}
}

// summaryWords are the labels a spreadsheet's own footer rows carry.
//
// A pharmacy's file usually ends with a total line, and i18n.TDefault("w4_ui.s_91_91") is not a
// product. Staging it would put it in front of the matcher, which would dutifully
// find the nearest medicine to the word "total".
var summaryWords = []string{
	i18n.TDefault("w4_ui.s_91_91"), i18n.TDefault("w4_ui.s_89_89"), i18n.TDefault("w4_mod.s_449_449"), i18n.TDefault("w4_ui.s_88_88"), i18n.TDefault("w4_mod.s_450_450"), i18n.TDefault("w4_ui.s_92_92"),
	"total", "subtotal", "grand total", "sum",
}

// IsSummaryRow reports whether a product cell is really a footer label.
func IsSummaryRow(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	for _, w := range summaryWords {
		if n == strings.ToLower(w) {
			return true
		}
	}
	return false
}
