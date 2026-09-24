package pages

import (
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
)

// Shared rendering helpers for the catalogue import screens.
//
// These describe how a file was read — which column became which field, and how
// sure the mapper was — and are used by the review step of the import wizard.

// ImportBindingRow is one column-to-field decision as rendered.
type ImportBindingRow struct {
	Column     string
	Header     string
	Field      string
	Confidence string
	// Sample is what that column actually holds. A binding is judged by its
	// values, not by its header: i18n.TDefault("w4m_ui.s_2_2") bound to a column of dates is obvious
	// here and invisible without it.
	Sample string
}

func delimiterLabel(d string) string {
	switch d {
	case "	":
		return "Tab"
	case "":
		return ","
	default:
		return d
	}
}

// importConfidenceBadge picks the badge colour for a mapping confidence.
//
// The words come from catalog.ConfidenceOf, which is the one place a score is
// turned into a judgement; a second vocabulary here drifted from it and painted
// every binding amber.
func importConfidenceBadge(confidence string) string {
	switch confidence {
	case catalog.ConfidenceOf(100):
		return "badge-emerald"
	case catalog.ConfidenceOf(60):
		return "badge-sky"
	default:
		return "badge-amber"
	}
}
