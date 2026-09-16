package catalog

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// readStatus applies the status column when the file supplies a value the
// products table's CHECK constraint accepts.
func (c rowCursor) readStatus(prod *Product) {
	raw := c.value(FieldStatus)
	if raw == "" {
		return
	}
	status, ok := CoerceStatus(raw)
	if !ok {
		c.warn(c.label(FieldStatus), raw,
			"حالة الصنف غير معروفة؛ تم تجاهل العمود والإبقاء على الحالة الافتراضية للصنف.")
		return
	}
	prod.Status = status
}

// guessNameCell finds the cell most likely to be a product name in a row whose
// name column is missing or mis-mapped.
func guessNameCell(row []string, plan ColumnPlan) string {
	identifierCols := map[int]bool{}
	for _, field := range []string{FieldSKU, FieldBarcode, FieldPrice, FieldPublicPrice,
		FieldCostPrice, FieldDiscount, FieldQuantity, FieldStatus, FieldUnit} {
		if idx, ok := plan.Columns[field]; ok {
			identifierCols[idx] = true
		}
	}

	best := ""
	for idx, cell := range row {
		if identifierCols[idx] {
			continue
		}
		clean := CleanCellString(cell)
		// Six characters excludes codes, dates and units without excluding a
		// short real name such as "بنادول".
		if len([]rune(clean)) < 6 || digitsOnlyPattern.MatchString(NormalizeDigits(clean)) {
			continue
		}
		if len([]rune(clean)) > len([]rune(best)) {
			best = clean
		}
	}
	return best
}

// dedupeKey picks the identity a row carries. SKU is the primary unique identifier;
// a normalised name with manufacturer is the fallback. Barcode is intentionally not
// used as a uniqueness key so multiple products or packages sharing a barcode are
// preserved and not wrongly collapsed.
func dedupeKey(p *Product) string {
	if sku := strings.ToLower(strings.TrimSpace(p.SKU)); sku != "" {
		return "sku:" + sku
	}
	name := NormalizeName(p.Name.Get(i18n.AR))
	if name == "" {
		name = NormalizeName(p.Name.Get(i18n.EN))
	}
	return "name:" + name + "|" + NormalizeKey(p.ManufacturingCompanies)
}

// mergeProduct folds a later duplicate row into the one already kept, filling
// gaps without overwriting values that were already supplied. A supplier who
// lists a product twice — once with a price, once with a barcode — ends up with
// one complete row instead of two half-empty ones.
func mergeProduct(dst, src *Product) {
	fillString(&dst.SKU, src.SKU)
	fillString(&dst.Barcode, src.Barcode)
	fillString(&dst.ScientificName, src.ScientificName)
	fillString(&dst.Active, src.Active)
	fillString(&dst.Unit, src.Unit)
	fillString(&dst.ManufacturingCompanies, src.ManufacturingCompanies)
	fillString(&dst.Concentration, src.Concentration)

	// An inferred form is weaker evidence than a stated one, so a later row that
	// names the form outright replaces the placeholder.
	if dst.DosageForm == "" || dst.DosageForm == defaultDosageForm {
		if src.DosageForm != "" && src.DosageForm != defaultDosageForm {
			dst.DosageForm = src.DosageForm
		}
	}
	if dst.Price.IsZero() {
		dst.Price = src.Price
	}
	if dst.OldPrice.IsZero() {
		dst.OldPrice = src.OldPrice
	}
	if dst.Discount.IsZero() {
		dst.Discount = src.Discount
	}
	if dst.Status == "" {
		dst.Status = src.Status
	}
	if dst.Name.Get(i18n.EN) == "" && src.Name.Get(i18n.EN) != "" {
		dst.Name[i18n.EN] = src.Name.Get(i18n.EN)
	}
	if dst.Description.IsEmpty() && !src.Description.IsEmpty() {
		dst.Description = src.Description
	}
}

func fillString(dst *string, src string) {
	if *dst == "" {
		*dst = src
	}
}

// crossFillIdentifiers fills an empty barcode from SKU when available, without forcing
// barcode into SKU.
func crossFillIdentifiers(prods []*Product) {
	for _, p := range prods {
		if p.Barcode == "" && p.SKU != "" {
			p.Barcode = p.SKU
		}
	}
}

// isBlankRow reports whether every cell is empty.
func isBlankRow(row []string) bool {
	for _, cell := range row {
		if CleanCellString(cell) != "" {
			return false
		}
	}
	return true
}

// normalizedKeySet folds a header row into a set for repeated-header matching.
func normalizedKeySet(header []string) map[string]bool {
	set := make(map[string]bool, len(header))
	for _, cell := range header {
		if key := NormalizeKey(cell); key != "" {
			set[key] = true
		}
	}
	return set
}

// isRepeatedHeader reports whether a data row is really the header printed
// again.
//
// Distributor systems paginate their exports and reprint the column titles every
// page — one real file carried 114 of them. Matching against the detected header
// generalises past the previous hardcoded list of nine literal strings, which
// silently imported every reprinted header of every file whose titles were not
// on that list as if it were a product.
func isRepeatedHeader(row []string, headerKeys map[string]bool) bool {
	if len(headerKeys) == 0 {
		return false
	}
	filled, matched := 0, 0
	for _, cell := range row {
		key := NormalizeKey(cell)
		if key == "" {
			continue
		}
		filled++
		if headerKeys[key] {
			matched++
		}
	}
	// Two thirds agreement, and at least two cells, so a product legitimately
	// named after a column ("سعر") cannot trip it on its own.
	return filled >= 2 && matched*3 >= filled*2
}

// missingImportantFields lists the fields worth telling the admin were absent.
// Their absence is not an error — plenty of valid files carry only names and
// prices — but a silently missing price column is how a catalogue ends up
// entirely free of charge.
func missingImportantFields(plan ColumnPlan) []string {
	var missing []string
	// Price is satisfied by any of the price columns, because row parsing falls
	// back from the selling price to the public price. Reporting "السعر مفقود"
	// for a file that plainly carries "سعر البيع للجمهور" trains admins to
	// ignore the warnings, which is worse than not showing them.
	if !plan.Has(FieldPrice) && !plan.Has(FieldPublicPrice) {
		missing = append(missing, FieldLabels[FieldPrice])
	}
	for _, field := range []string{FieldNameAR, FieldSKU, FieldManufacturer} {
		if !plan.Has(field) {
			missing = append(missing, FieldLabels[field])
		}
	}
	return missing
}

// addIssue records an issue, keeping the counters exact even once the retained
// list is full.
func (r *ParseResult) addIssue(issue RowIssue) {
	if issue.Severity == SeverityWarning {
		r.Stats.Warnings++
	}
	if len(r.Issues) < maxIssues {
		r.Issues = append(r.Issues, issue)
	}
}

// ParseProductRows converts raw spreadsheet rows into cleaned, valid products.
//
// Retained as the narrow entry point for callers that need only the products;
// ParseProducts carries the per-row issues the import report renders.
func ParseProductRows(records [][]string) ([]*Product, ImportStats) {
	data := &SheetData{Rows: records}
	normalizeWidth(data)
	res := ParseProducts(data)
	return res.Products, res.Stats
}
