package ui

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/arabic"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

var scientificNotationRegex = regexp.MustCompile(`(?i)^[0-9]+(\.[0-9]+)?e\+[0-9]+$`)

// NormalizeDigitsOnly converts Arabic-Indic digits ٠-٩ to 0-9 without altering
// periods, commas, or letters.
func NormalizeDigitsOnly(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= '٠' && r <= '٩':
			b.WriteRune('0' + (r - '٠'))
		case r >= '۰' && r <= '۹':
			b.WriteRune('0' + (r - '۰'))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SanitizeSKUCode cleans the artefacts a spreadsheet adds to a code: Arabic
// digits, scientific notation, a trailing ".0", and separator noise.
func SanitizeSKUCode(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = NormalizeDigitsOnly(s)

	if scientificNotationRegex.MatchString(s) {
		if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			s = fmt.Sprintf("%.0f", f)
		}
	}
	s = strings.TrimSuffix(s, ".0")

	s = strings.ToLower(s)
	for _, noise := range []string{"-", " ", "_", "/"} {
		s = strings.ReplaceAll(s, noise, "")
	}
	if isNumericOnly(s) {
		if trimmed := strings.TrimLeft(s, "0"); trimmed != "" {
			return trimmed
		}
	}
	return s
}

func isNumericOnly(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// ParseFlexibleQuantity parses the quantity formats a spreadsheet produces.
func ParseFlexibleQuantity(raw string) (float64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}
	s = NormalizeDigitsOnly(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, " ", "")

	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return 0, false
	}
	return f, true
}

// ParseFlexibleMoney parses a price carrying a currency word or thousands separators.
func ParseFlexibleMoney(raw string) (money.Amount, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return money.Amount{}, false
	}
	s = NormalizeDigitsOnly(s)
	s = strings.ReplaceAll(s, ",", "")
	for _, unit := range []string{i18n.TDefault("w4_ui.s_50_50"), i18n.TDefault("w4_ui.s_51_51"), i18n.TDefault("w4_ui.s_87_87"), "egp", "EGP", "le", "LE", "$"} {
		s = strings.ReplaceAll(s, unit, "")
	}
	s = strings.TrimSpace(s)

	amt, err := money.Parse(s)
	if err != nil {
		f, ferr := strconv.ParseFloat(s, 64)
		if ferr != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
			return money.Amount{}, false
		}
		return money.FromMinor(int64(math.Round(f * 100))), true
	}
	if amt.IsNegative() {
		return money.Amount{}, false
	}
	return amt, true
}

// summaryWords are the labels a spreadsheet's footer uses.
var summaryWords = []string{
	i18n.TDefault("w4_ui.s_88_88"), i18n.TDefault("w4_ui.s_89_89"), i18n.TDefault("w4_ui.s_90_90"), i18n.TDefault("w4_ui.s_91_91"), i18n.TDefault("w4_ui.s_92_92"), i18n.TDefault("w4_ui.s_93_93"),
	"total", "grand total", "subtotal", "sum", "summary", "count",
}

// IsSummaryOrTotalRow reports whether a row is a footer rather than a product.
func IsSummaryOrTotalRow(row []string) bool {
	if len(row) == 0 {
		return true
	}
	for _, cell := range row {
		clean := strings.TrimSpace(strings.ToLower(cell))
		if clean == "" {
			continue
		}
		norm := arabic.Normalize(clean)
		for _, w := range summaryWords {
			if norm == w || strings.HasPrefix(norm, w+" ") || strings.HasPrefix(norm, w+":") {
				return true
			}
		}
		break
	}
	return false
}

// IsAllEmptyRow reports whether every cell is blank.
func IsAllEmptyRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}
