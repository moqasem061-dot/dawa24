package ui

import (
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Reading the columns of a saving-products file.
//
// This used to be a private detector that walked the headers left to right and
// gave each column to the first field whose keyword it contained. It carried
// exactly the bug productmatch was written to eliminate: the item-code rule
// tested for i18n.TDefault("w4_ui.s_2_2"), which is a substring of i18n.TDefault("w4_ui.s_3_3"), and it ran before the
// name rule — so a column headed i18n.TDefault("w4_ui.s_84_84") became the item code and the real
// code column, further right, was never looked at. Whatever the header rules
// left unassigned was then handed the next unused column outright, which is how
// a price column could be read as a product name.
//
// It now asks the same resolver the vendor, catalogue and smart-order imports
// ask, narrowed to the five fields this file can carry. Two witnesses instead
// of one: what the header claims, and what the values actually look like, with
// a veto on any binding the values contradict.

// detectSavingProductColumns resolves the five columns a saving-products file
// can carry, honouring any column the user pinned by hand.
//
// A returned -1 means "not found", and the caller renders the mapping screen
// asking for it. That is deliberately different from the previous behaviour,
// which guessed: an unmapped column a person can fix in one click beats a wrong
// one they have to notice first.
func detectSavingProductColumns(
	headers []string,
	sampleRows [][]string,
	customName, customSKU, customQty, customPrice, customProductID string,
) (nameCol, skuCol, qtyCol, priceCol, productIDCol int) {
	numCols := len(headers)
	nameCol = parseColOverride(customName, numCols)
	skuCol = parseColOverride(customSKU, numCols)
	qtyCol = parseColOverride(customQty, numCols)
	priceCol = parseColOverride(customPrice, numCols)
	productIDCol = parseColOverride(customProductID, numCols)

	mapping := resolveSavingColumns(headers, sampleRows)
	if mapping != nil {
		assign := func(current int, f productmatch.Field) int {
			if current >= 0 {
				return current // the user pinned this one
			}
			if col, ok := mapping.Column(f); ok {
				return col
			}
			return -1
		}
		productIDCol = assign(productIDCol, productmatch.FieldProductID)
		nameCol = assign(nameCol, productmatch.FieldName)
		skuCol = assign(skuCol, productmatch.FieldSKU)
		qtyCol = assign(qtyCol, productmatch.FieldQuantity)

		// Whichever price the file states is the price this entry records.
		// Preference order is what a saving list means by "the price": what the
		// public pays, else what this buyer pays, else what it cost.
		for _, f := range []productmatch.Field{
			productmatch.FieldPublicPrice, productmatch.FieldPrice,
			productmatch.FieldNetPrice, productmatch.FieldCostPrice,
		} {
			if priceCol = assign(priceCol, f); priceCol >= 0 {
				break
			}
		}

		// A file may carry a barcode instead of an item code; both resolve a
		// row to a catalogue product, and the matcher treats them separately,
		// so the barcode is accepted here only when there is no item code to
		// put in this slot.
		if skuCol < 0 {
			skuCol = assign(skuCol, productmatch.FieldBarcode)
		}
	}
	return nameCol, skuCol, qtyCol, priceCol, productIDCol
}

// resolveSavingColumns profiles the sample and runs the shared resolver.
func resolveSavingColumns(headers []string, sampleRows [][]string) *productmatch.Mapping {
	if len(headers) == 0 {
		return nil
	}
	profiles := make([]*sheet.ColumnProfile, len(headers))
	for i, h := range headers {
		profiles[i] = sheet.NewColumnProfile(i, h)
	}
	for _, row := range sampleRows {
		for i := range profiles {
			if i < len(row) {
				profiles[i].Observe(row[i])
			} else {
				profiles[i].Observe("")
			}
		}
	}
	for _, p := range profiles {
		p.Seal()
	}
	return productmatch.ResolveWith(headers, profiles, nil, productmatch.SavingFields)
}

// parseColOverride reads a column index the user pinned on the mapping screen.
func parseColOverride(val string, numCols int) int {
	val = strings.TrimSpace(val)
	if val == "" {
		return -1
	}
	if idx, err := strconv.Atoi(val); err == nil && idx >= 0 && idx < numCols {
		return idx
	}
	return -1
}
