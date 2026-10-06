package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
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

	detected := compare.DetectColumns(headers)
	used := map[int]bool{codeCol: codeCol >= 0, nameCol: nameCol >= 0, priceCol: priceCol >= 0, discountCol: discountCol >= 0}
	for column := range headers {
		field, ok := detected[column]
		if !ok {
			continue
		}
		if used[column] {
			continue
		}
		switch field {
		case compare.FieldSKU, compare.FieldBarcode, compare.FieldProductID:
			if codeCol < 0 {
				codeCol = column
				used[column] = true
			}
		case compare.FieldProductName:
			if nameCol < 0 {
				nameCol = column
				used[column] = true
			}
		case compare.FieldPrice:
			if priceCol < 0 {
				priceCol = column
				used[column] = true
			}
		case compare.FieldDiscount:
			if discountCol < 0 {
				discountCol = column
				used[column] = true
			}
		}
	}

	return codeCol, nameCol, priceCol, discountCol
}

func validTempWarehouseMapping(cfg compare.MappingConfig) bool {
	return cfg.NameCol != nil && *cfg.NameCol >= 0 &&
		(cfg.PriceCol != nil || cfg.DiscountCol != nil) && !hasDuplicateTempWarehouseColumns(cfg)
}

func hasDuplicateTempWarehouseColumns(cfg compare.MappingConfig) bool {
	used := make([]int, 0, 4)
	for _, column := range []*int{cfg.CodeCol, cfg.NameCol, cfg.PriceCol, cfg.DiscountCol} {
		if column == nil {
			continue
		}
		if slices.Contains(used, *column) {
			return true
		}
		used = append(used, *column)
	}
	return false
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
