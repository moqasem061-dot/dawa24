package compare

import (
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"strings"
	"unicode"

	"github.com/muhiya/dawa24-store/internal/shared/arabic"
)

// TargetField represents a canonical field needed by the comparison engine from a spreadsheet.
type TargetField string

const (
	FieldProductID     TargetField = "product_id"
	FieldProductName   TargetField = "product_name"
	FieldDescription   TargetField = "description"
	FieldPrice         TargetField = "price"
	FieldCostPrice     TargetField = "cost_price"
	FieldDiscount      TargetField = "discount"
	FieldQuantity      TargetField = "quantity"
	FieldSKU           TargetField = "sku"
	FieldUniqueID      TargetField = "unique_id"
	FieldBarcode       TargetField = "barcode"
	FieldAlertPrice    TargetField = "alert_price"
	FieldAlertDiscount TargetField = "alert_discount"
)

// ArabicLabel returns human-readable Arabic name for a target field.
func ArabicLabel(f TargetField) string {
	switch f {
	case FieldProductID:
		return i18n.TDefault("w4_mod.s_365_365")
	case FieldProductName:
		return i18n.TDefault("w4_ui.s_53_53")
	case FieldDescription:
		return i18n.TDefault("w4_mod.s_366_366")
	case FieldPrice:
		return i18n.TDefault("w4_ui.s_54_54")
	case FieldCostPrice:
		return i18n.TDefault("w4_mod.s_367_367")
	case FieldDiscount:
		return i18n.TDefault("w4_ui.s_55_55")
	case FieldQuantity:
		return i18n.TDefault("w4_mod.s_368_368")
	case FieldSKU:
		return i18n.TDefault("w4_mod.s_369_369")
	case FieldUniqueID:
		return i18n.TDefault("w4_mod.s_370_370")
	case FieldBarcode:
		return i18n.TDefault("w4_ui.s_84_84")
	case FieldAlertPrice:
		return i18n.TDefault("w4_mod.s_371_371")
	case FieldAlertDiscount:
		return i18n.TDefault("w4_mod.w4str_159_159")
	default:
		return string(f)
	}
}

// CleanHeader strips BOM, zero-width characters, and leading/trailing whitespace.
func CleanHeader(value string) string {
	// Remove UTF-8 BOM
	value = strings.TrimPrefix(value, "\xEF\xBB\xBF")
	// Remove zero-width spaces and control characters
	var b strings.Builder
	for _, r := range value {
		if r == '\u200B' || r == '\u200C' || r == '\u200D' || r == '\uFEFF' {
			continue
		}
		if !unicode.IsControl(r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// HeaderMatch holds the field and confidence score for a detected header column.
type HeaderMatch struct {
	Field      TargetField
	Confidence float64
}

// DetectColumns proposes a header index → target field mapping for the given header row.
// It is a deterministic, pure function with no I/O (Plan V5 Phase 2 §2.3.2).
func DetectColumns(headers []string) map[int]TargetField {
	mapping := make(map[int]TargetField)
	usedFields := make(map[TargetField]bool)

	for idx, rawHeader := range headers {
		cleaned := CleanHeader(rawHeader)
		if cleaned == "" {
			continue
		}

		match := findBestMatch(cleaned)
		if match != nil && !usedFields[match.Field] {
			mapping[idx] = match.Field
			usedFields[match.Field] = true
		}
	}

	return mapping
}

// DetectColumnsWithConfidence returns detailed matches including confidence scores.
func DetectColumnsWithConfidence(headers []string) (map[int]TargetField, map[TargetField]float64, float64) {
	mapping := make(map[int]TargetField)
	confidenceScores := make(map[TargetField]float64)
	usedFields := make(map[TargetField]bool)

	for idx, rawHeader := range headers {
		cleaned := CleanHeader(rawHeader)
		if cleaned == "" {
			continue
		}

		match := findBestMatch(cleaned)
		if match != nil && !usedFields[match.Field] {
			mapping[idx] = match.Field
			confidenceScores[match.Field] = match.Confidence
			usedFields[match.Field] = true
		}
	}

	overallConfidence := calculateOverallConfidence(confidenceScores)
	return mapping, confidenceScores, overallConfidence
}

func findBestMatch(header string) *HeaderMatch {
	normHeader := normalizeForComparison(header)
	if normHeader == "" {
		return nil
	}

	var bestMatch *HeaderMatch
	highestScore := 0.0

	for field, aliases := range HeaderAliases {
		for _, alias := range aliases {
			normAlias := normalizeForComparison(alias)
			score := calculateSimilarity(normHeader, normAlias)

			if score > highestScore {
				highestScore = score
				bestMatch = &HeaderMatch{
					Field:      field,
					Confidence: score,
				}
			}

			// Exact match terminates early
			if score == 1.0 {
				return bestMatch
			}
		}
	}

	// Matching threshold > 0.65 per Laravel ColumnDetector.php
	if highestScore > 0.65 {
		return bestMatch
	}
	return nil
}

func normalizeForComparison(s string) string {
	s = arabic.Normalize(s)
	s = strings.ToLower(strings.TrimSpace(s))
	// Strip special punctuation
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func calculateSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	if s1 == "" || s2 == "" {
		return 0.0
	}

	// Substring / containment bonus
	if strings.Contains(s1, s2) || strings.Contains(s2, s1) {
		lenMin := min(len(s1), len(s2))
		lenMax := max(len(s1), len(s2))
		ratio := float64(lenMin) / float64(lenMax)
		return 0.70 + (0.30 * ratio)
	}

	// Character bigram similarity (Dice coefficient)
	bigrams1 := getBigrams(s1)
	bigrams2 := getBigrams(s2)

	if len(bigrams1) == 0 || len(bigrams2) == 0 {
		return 0.0
	}

	intersection := 0
	for bg := range bigrams1 {
		if bigrams2[bg] {
			intersection++
		}
	}

	return (2.0 * float64(intersection)) / float64(len(bigrams1)+len(bigrams2))
}

func getBigrams(s string) map[string]bool {
	bg := make(map[string]bool)
	runes := []rune(s)
	for i := 0; i < len(runes)-1; i++ {
		bg[string(runes[i:i+2])] = true
	}
	return bg
}

func calculateOverallConfidence(scores map[TargetField]float64) float64 {
	if len(scores) == 0 {
		return 0.0
	}

	_, hasName := scores[FieldProductName]
	_, hasPrice := scores[FieldPrice]
	_, hasDiscount := scores[FieldDiscount]

	if !hasName || (!hasPrice && !hasDiscount) {
		return 0.3
	}

	sum := 0.0
	for _, score := range scores {
		sum += score
	}
	avg := sum / float64(len(scores))

	// Bonus for detecting more fields (up to 0.2)
	bonus := float64(len(scores)) / 10.0
	if bonus > 0.2 {
		bonus = 0.2
	}

	total := avg + bonus
	if total > 1.0 {
		total = 1.0
	}
	return total
}

// FindBestHeaderRow scans the top rows of a spreadsheet to find the most probable header row.
func FindBestHeaderRow(rows [][]string) (int, map[int]TargetField, float64) {
	if len(rows) == 0 {
		return 0, nil, 0.0
	}
	bestIndex := -1
	bestConfidence := 0.0
	var bestMapping map[int]TargetField

	for i, row := range rows {
		if len(row) == 0 {
			continue
		}
		// Skip entirely blank rows
		hasContent := false
		for _, cell := range row {
			if strings.TrimSpace(cell) != "" {
				hasContent = true
				break
			}
		}
		if !hasContent {
			continue
		}

		mapping, _, confidence := DetectColumnsWithConfidence(row)
		hasName := false
		for _, f := range mapping {
			if f == FieldProductName {
				hasName = true
				break
			}
		}

		if hasName && confidence > bestConfidence {
			bestConfidence = confidence
			bestIndex = i
			bestMapping = mapping
		}
	}

	if bestIndex != -1 && bestConfidence > 0.3 {
		return bestIndex, bestMapping, bestConfidence
	}
	if len(rows) > 0 {
		return 0, DetectColumns(rows[0]), 0.0
	}
	return 0, nil, 0.0
}

// ValidateMapping checks if the mapping contains required columns (product_name and at least price or discount).
func ValidateMapping(mapping map[int]TargetField) (bool, []TargetField) {
	hasName := false
	hasPriceOrDiscount := false

	for _, f := range mapping {
		if f == FieldProductName {
			hasName = true
		}
		if f == FieldPrice || f == FieldDiscount {
			hasPriceOrDiscount = true
		}
	}

	var missing []TargetField
	if !hasName {
		missing = append(missing, FieldProductName)
	}
	if !hasPriceOrDiscount {
		missing = append(missing, FieldPrice)
	}

	return len(missing) == 0, missing
}
