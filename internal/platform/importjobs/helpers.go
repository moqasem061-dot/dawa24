package importjobs

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/muhiya/dawa24-store/internal/shared/arabic"
)

// matchProduct is a lightweight catalogue entry for matching.
type matchProduct struct {
	ID   int64
	Name string
	SKU  string
}

// findBestMatch finds the best-matching catalogue product by name similarity.
func findBestMatch(name, sku string, products []matchProduct) (int64, float64, string, string) {
	normalized := arabic.Normalize(name)
	var bestID int64
	var bestScore float64
	var bestName, bestSKU string

	// If we have a SKU, try exact SKU match first.
	if sku != "" {
		cleanSKU := strings.TrimSpace(strings.ToLower(sku))
		for _, p := range products {
			if strings.TrimSpace(strings.ToLower(p.SKU)) == cleanSKU {
				return p.ID, 1.0, p.Name, p.SKU
			}
		}
	}

	// Fall back to name similarity.
	for _, p := range products {
		score := arabic.Similarity(normalized, arabic.Normalize(p.Name))
		if score > bestScore {
			bestScore = score
			bestID = p.ID
			bestName = p.Name
			bestSKU = p.SKU
		}
	}

	return bestID, bestScore, bestName, bestSKU
}

// isAllEmpty returns true if every cell in a row is empty.
func isAllEmpty(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

// isSummaryRow detects total/summary rows that should be skipped.
func isSummaryRow(row []string) bool {
	if len(row) == 0 {
		return false
	}
	first := strings.TrimSpace(row[0])
	lower := strings.ToLower(first)
	switch {
	case strings.HasPrefix(lower, "total"),
		strings.HasPrefix(lower, "الإجمالي"),
		strings.HasPrefix(lower, "اجمالي"),
		strings.HasPrefix(lower, "المجموع"),
		strings.HasPrefix(first, "---"):
		return true
	}
	return false
}

// isAllDigitsOrCode returns true if the string looks like a numeric code.
func isAllDigitsOrCode(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) && r != '-' && r != '/' && r != '.' {
			return false
		}
	}
	return true
}

// isDescriptiveText returns true if the string contains Arabic letters.
func isDescriptiveText(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Arabic, r) {
			return true
		}
	}
	return false
}

// parseFlexQty parses a flexible quantity string.
func parseFlexQty(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "٫", ".")
	// Replace Arabic-Indic numerals.
	var sb strings.Builder
	for _, r := range s {
		if r >= '٠' && r <= '٩' {
			sb.WriteRune('0' + (r - '٠'))
		} else {
			sb.WriteRune(r)
		}
	}
	v, _ := strconv.ParseFloat(sb.String(), 64)
	return v
}
