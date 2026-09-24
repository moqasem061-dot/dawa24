package compare

import (
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// IsValidProductRow returns true if the row represents an actual product item,
// filtering out empty rows, rows with only punctuation/dashes/spaces, repeated headers, and report noise.
func IsValidProductRow(name, sku string, price money.Amount, discount float64) bool {
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		return false
	}

	// Reject if name consists only of punctuation, dashes, dots, asterisks, slashes, spaces
	hasAlphaNum := false
	for _, r := range cleanName {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			(r >= 0x0600 && r <= 0x06FF) || (r >= 0x0750 && r <= 0x077F) {
			hasAlphaNum = true
			break
		}
	}
	if !hasAlphaNum {
		return false
	}

	// Reject single character names (accidental typo/noise)
	runes := []rune(cleanName)
	if len(runes) < 2 {
		return false
	}

	// Check for common header row keywords that leak into rows
	lowerName := strings.ToLower(cleanName)
	headerKeywords := []string{
		"اسم الصنف", "اسم المنتج", "اسم الدواء", "الاسم التجاري", "بيان الصنف", "الصنف", "المستحضر",
		"كود الصنف", "كود المنتج", "رقم الصنف", "الباركود", "باركود",
		"السعر", "سعر الجمهور", "سعر البيع", "سعر الصيدلي", "السعر الرسمي",
		"الخصم", "نسبة الخصم", "الخصم الإضافي", "الصافي", "السعر بعد الخصم",
		"product name", "item name", "medicine name", "drug name", "description", "item description",
		"sku", "item code", "barcode", "price", "unit price", "cost", "discount", "net price",
	}
	for _, kw := range headerKeywords {
		if lowerName == kw || strings.Trim(lowerName, ": -_#.") == kw {
			return false
		}
	}

	// If price is zero, discount is zero, and SKU is empty, check if it's just a section divider or page header
	if price.IsZero() && discount <= 0 && strings.TrimSpace(sku) == "" {
		if strings.HasPrefix(lowerName, "صفحة") || strings.HasPrefix(lowerName, "page") ||
			strings.HasPrefix(lowerName, "total") || strings.HasPrefix(lowerName, "المجموع") ||
			strings.HasPrefix(lowerName, "الإجمالي") || strings.HasPrefix(lowerName, "الاجمالي") {
			return false
		}
	}

	return true
}
