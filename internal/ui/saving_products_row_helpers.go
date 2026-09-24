package ui

import (
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// IsJunkOrEmptyRow reports whether a row is empty, pure punctuation, or a repeated header.
func IsJunkOrEmptyRow(row []string) bool {
	if len(row) == 0 || IsAllEmptyRow(row) || IsSummaryOrTotalRow(row) {
		return true
	}
	hasAlphaNum := false
	for _, cell := range row {
		for _, r := range cell {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
				(r >= 0x0600 && r <= 0x06FF) || (r >= 0x0750 && r <= 0x077F) {
				hasAlphaNum = true
				break
			}
		}
		if hasAlphaNum {
			break
		}
	}
	return !hasAlphaNum
}

// IsValidSavingRow checks whether a row has a valid product name, rejecting headers, punctuation, and junk.
func IsValidSavingRow(name, sku string) bool {
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		return false
	}
	return compare.IsValidProductRow(cleanName, sku, money.Zero, 0)
}
