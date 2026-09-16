package pages

import (
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
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

// describeSource says which worksheet or delimiter the rows came from, so an
// admin whose workbook has five tabs can confirm the right one was read.
func describeSource(stats catalog.ImportStats) string {
	switch stats.Format {
	case "xlsx":
		if stats.SheetName != "" {
			return fmt.Sprintf(i18n.TDefault("w4_ui.excel_s_36"), stats.SheetName)
		}
		return i18n.TDefault("w4_ui.excel_37")
	case "csv":
		return fmt.Sprintf(i18n.TDefault("w4_ui.s_38"), delimiterLabel(stats.Delimiter))
	default:
		return ""
	}
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

// columnLetter renders a zero-based column index the way Excel labels it, so a
// finding can be found in the file without counting columns by hand.
func columnLetter(idx int) string {
	if idx < 0 {
		return "—"
	}
	name := ""
	for n := idx; ; n = n/26 - 1 {
		name = string(rune('A'+n%26)) + name
		if n < 26 {
			break
		}
	}
	return name
}
