package ui

import (
	"slices"
	"strconv"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
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
