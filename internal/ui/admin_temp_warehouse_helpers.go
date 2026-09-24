package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// detectTempWarehouseCols determines column indices based on header names or custom inputs.
func detectTempWarehouseCols(headers []string, customCode, customName, customPrice, customDiscount string) (codeCol, nameCol, priceCol, discountCol int) {
	codeCol, nameCol, priceCol, discountCol = -1, -1, -1, -1
	numCols := len(headers)

	if customCode != "" {
		if c, err := strconv.Atoi(customCode); err == nil && c >= 0 && c < numCols {
			codeCol = c
		}
	}
	if customName != "" {
		if c, err := strconv.Atoi(customName); err == nil && c >= 0 && c < numCols {
			nameCol = c
		}
	}
	if customPrice != "" {
		if c, err := strconv.Atoi(customPrice); err == nil && c >= 0 && c < numCols {
			priceCol = c
		}
	}
	if customDiscount != "" {
		if c, err := strconv.Atoi(customDiscount); err == nil && c >= 0 && c < numCols {
			discountCol = c
		}
	}

	for i, h := range headers {
		norm := strings.ToLower(strings.TrimSpace(h))
		norm = strings.ReplaceAll(norm, "_", "")
		norm = strings.ReplaceAll(norm, "-", "")
		norm = strings.ReplaceAll(norm, " ", "")

		if codeCol == -1 {
			if strings.Contains(norm, i18n.TDefault("w4_ui.s_2_2")) || strings.Contains(norm, "code") || strings.Contains(norm, "sku") ||
				strings.Contains(norm, i18n.TDefault("w4_ui.s_3_3")) || strings.Contains(norm, "barcode") || strings.Contains(norm, i18n.TDefault("w4_ui.s_29_29")) ||
				strings.Contains(norm, "itemcode") {
				codeCol = i
			}
		}

		if nameCol == -1 {
			if strings.Contains(norm, i18n.TDefault("w4_ui.s_30_30")) || strings.Contains(norm, "name") || strings.Contains(norm, i18n.TDefault("w4_ui.s_31_31")) ||
				strings.Contains(norm, i18n.TDefault("w4_ui.s_32_32")) || strings.Contains(norm, "item") || strings.Contains(norm, "product") {
				nameCol = i
			}
		}

		if priceCol == -1 {
			if (strings.Contains(norm, i18n.TDefault("w4_ui.s_33_33")) || strings.Contains(norm, "price") || strings.Contains(norm, i18n.TDefault("w4_ui.s_34_34")) || strings.Contains(norm, i18n.TDefault("w4_ui.s_35_35"))) && !strings.Contains(norm, i18n.TDefault("w4_ui.s_36_36")) && !strings.Contains(norm, i18n.TDefault("w4_ui.s_37_37")) {
				priceCol = i
			}
		}

		if discountCol == -1 {
			if strings.Contains(norm, i18n.TDefault("w4_ui.s_36_36")) || strings.Contains(norm, "discount") || strings.Contains(norm, "%") || strings.Contains(norm, i18n.TDefault("w4_ui.s_38_38")) {
				discountCol = i
			}
		}
	}

	if codeCol == -1 && numCols > 0 {
		codeCol = 0
	}
	if nameCol == -1 && numCols > 1 {
		nameCol = 1
	}
	if priceCol == -1 && numCols > 2 {
		priceCol = 2
	}
	if discountCol == -1 && numCols > 3 {
		discountCol = 3
	}

	return codeCol, nameCol, priceCol, discountCol
}

// parsePriceFloat parses a numeric string into float64, converting Arabic numerals and commas.
func parsePriceFloat(raw string) (float64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_39_39"), "0")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_40_40"), "1")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_41_41"), "2")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_42_42"), "3")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_43_43"), "4")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_44_44"), "5")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_45_45"), "6")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_46_46"), "7")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_47_47"), "8")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_48_48"), "9")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_49_49"), ".")
	s = strings.ReplaceAll(s, "%", "")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_50_50"), "")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_51_51"), "")
	s = strings.ReplaceAll(s, "EGP", "")
	s = strings.ReplaceAll(s, "egp", "")
	s = strings.ReplaceAll(s, "LE", "")
	s = strings.ReplaceAll(s, "le", "")
	s = strings.TrimSpace(s)

	if strings.Contains(s, ",") && strings.Contains(s, ".") {
		s = strings.ReplaceAll(s, ",", "")
	} else if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ",", ".")
	}

	var numStr strings.Builder
	hasDot := false
	foundDigit := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			numStr.WriteRune(r)
			foundDigit = true
		} else if r == '.' && !hasDot {
			numStr.WriteRune(r)
			hasDot = true
		} else if foundDigit && r != ' ' && r != '\t' {
			break
		}
	}
	if !foundDigit {
		return 0, fmt.Errorf("no digit found")
	}
	return strconv.ParseFloat(numStr.String(), 64)
}
