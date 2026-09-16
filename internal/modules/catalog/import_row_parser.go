package catalog

import (
	"fmt"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// collectProducts walks every block, skipping the noise and folding in-file
// duplicates together.
//
// Reading block by block is what lets a sheet whose second section adds a price
// column be read correctly: each block is interpreted through its own header
// rather than through the first block's mapping.
func (r *ParseResult) collectProducts(records [][]string) []*Product {
	// seen keys a product by the strongest identifier its row carried, so two
	// rows sharing a SKU merge even when their names differ by a typo.
	seen := make(map[string]*Product, r.Layout.DataRows)
	var products []*Product

	for _, block := range r.Layout.Blocks {
		headerKeys := map[string]bool{}
		if block.HeaderRow >= 0 && block.HeaderRow < len(records) {
			headerKeys = normalizedKeySet(records[block.HeaderRow])
		}

		for rIdx := block.FirstRow; rIdx <= block.LastRow && rIdx < len(records); rIdx++ {
			row := records[rIdx]

			if isBlankRow(row) {
				r.Stats.EmptyRows++
				continue
			}
			if isRepeatedHeader(row, headerKeys) {
				r.Stats.RepeatedHeader++
				continue
			}

			// Spreadsheet row numbers are 1-based and this is what the admin
			// sees in Excel's row gutter, so every issue is reported against it.
			cursor := rowCursor{result: r, plan: block.Plan, row: row, number: rIdx + 1, block: block.Index}
			prod, ok := cursor.parse()
			if !ok {
				r.Stats.RejectedRows++
				continue
			}

			key := dedupeKey(prod)
			if existing, isDuplicate := seen[key]; isDuplicate {
				mergeProduct(existing, prod)
				r.Stats.DuplicateRows++
				// The finding is reported against the FIRST occurrence's row,
				// the only one of the two that becomes a staged row — a warning
				// attached to the duplicate's row number never reached the
				// review table, and the admin was never told their file had
				// merged rows at all.
				firstRow := 0
				if n := len(r.SourceRows); n > 0 {
					firstRow = r.SourceRows[n-1]
				}
				r.addIssue(RowIssue{
					Row: firstRow, Value: prod.Name.Get(i18n.AR),
					Message: fmt.Sprintf(
						"صنف مكرر داخل الملف نفسه (الصف %d)؛ تم دمج بياناته مع الصف %d بدلاً من تكراره في الكتالوج.",
						rIdx+1, firstRow),
					Severity: SeverityWarning,
				})
				continue
			}

			seen[key] = prod
			products = append(products, prod)
			r.SourceRows = append(r.SourceRows, rIdx+1)
		}
	}

	crossFillIdentifiers(products)
	return products
}

// rowCursor addresses one spreadsheet row through the resolved column plan.
//
// Bundling the row with its number and the plan is what keeps the readers below
// down to one or two parameters: without it every one of them has to be handed a
// value accessor, a label accessor and a row number, and the row itself rides
// along unused because the accessors already close over it.
type rowCursor struct {
	result *ParseResult
	// plan is the owning block's column mapping, which is not necessarily the
	// primary one: a sheet may stack sections of different shapes.
	plan   ColumnPlan
	row    []string
	number int
	block  int
}

// value returns a field's cell for this row, or empty when the field is unmapped
// or the row is short.
func (c rowCursor) value(field string) string {
	idx, ok := c.plan.Columns[field]
	if !ok || idx < 0 || idx >= len(c.row) {
		return ""
	}
	return CleanCellString(c.row[idx])
}

// label names a field the way the admin's own file names it, falling back to the
// canonical Arabic label when the file supplied no header for it.
func (c rowCursor) label(field string) string {
	for _, b := range c.plan.Bindings {
		if b.Field == field {
			return b.Header
		}
	}
	return FieldLabels[field]
}

func (c rowCursor) warn(column, value, message string) {
	c.result.addIssue(RowIssue{
		Row: c.number, Column: column, Value: value,
		Message: message, Severity: SeverityWarning,
	})
}

func (c rowCursor) reject(column, value, message string) {
	c.result.addIssue(RowIssue{
		Row: c.number, Column: column, Value: value,
		Message: message, Severity: SeverityError,
	})
}

// parse turns this row into a product, or rejects it with reasons.
func (c rowCursor) parse() (*Product, bool) {
	nameAR, nameEN, ok := c.resolveNames()
	if !ok {
		return nil, false
	}

	// Status is left empty unless the file states one. The write path defaults
	// an empty status to active on insert and leaves it untouched on update, so
	// re-importing a supplier's price list cannot silently reactivate a product
	// an admin deliberately took off the catalogue.
	prod := &Product{
		SKU:                    c.value(FieldSKU),
		Barcode:                c.value(FieldBarcode),
		ScientificName:         c.value(FieldGenericName),
		Active:                 c.value(FieldActive),
		DosageForm:             c.value(FieldDosageForm),
		Concentration:          NormalizeDigits(c.value(FieldConcentration)),
		Unit:                   c.value(FieldUnit),
		ManufacturingCompanies: c.value(FieldManufacturer),
		InstitutionalWorkIDs:   []int64{},
		// The file's own category word, kept as text until the taxonomy pass
		// resolves it to a catalogue category id.
		SourceCategory: c.value(FieldCategory),

		// The name columns stay in their own language. Copying Arabic into the
		// English slot — which the previous importer did unconditionally — makes
		// every English search match Arabic text and makes the catalogue look
		// translated when it is not.
		Name:        i18n.New(nameAR, nameEN),
		Description: i18n.New(c.value(FieldDescriptionAR), c.value(FieldDescriptionEN)),
	}

	// Fill the form and strength from the name only where the file gave none,
	// and only where the admin asked for it. The concentration is not behind a
	// switch: it is read out of the name verbatim rather than inferred, so it
	// states what the supplier wrote instead of guessing on their behalf.
	autoDosage, autoConc := ExtractDosageAndConcentration(nameAR + " " + nameEN)
	if prod.DosageForm == "" && c.result.options.AssignDosageForm {
		prod.DosageForm = autoDosage
	}
	if prod.Concentration == "" {
		prod.Concentration = autoConc
	}

	if !c.readPrices(prod) {
		return nil, false
	}
	c.readStatus(prod)
	img := c.value(FieldImage)
	if img != "" {
		prod.Image = img
		prod.ImageLink = img
	}
	return prod, true
}

// resolveNames determines the product's Arabic and English names, recovering
// from a mis-mapped or missing name column where it can.
func (c rowCursor) resolveNames() (nameAR, nameEN string, ok bool) {
	nameAR = c.value(FieldNameAR)
	nameEN = c.value(FieldNameEN)
	if nameAR != "" || nameEN != "" {
		return nameAR, nameEN, true
	}

	// A file whose name column is mapped wrongly, or missing entirely, still
	// usually carries the name somewhere.
	if guess := guessNameCell(c.row, c.plan); guess != "" {
		c.warn("", guess, "لم يتم العثور على اسم الصنف في العمود المتوقع؛ تم استنتاجه من محتوى الصف.")
		return guess, "", true
	}

	identifier := c.value(FieldSKU)
	if identifier == "" {
		identifier = c.value(FieldBarcode)
	}
	if identifier == "" {
		c.reject("", "", "تم تجاهل الصف: لا يحتوي على اسم صنف ولا كود ولا باركود.")
		return "", "", false
	}

	// An identifier with no name is a real row with a missing label; keep it
	// rather than dropping stock the pharmacy owns, and flag it for review.
	c.warn("", identifier,
		"الصف لا يحتوي على اسم صنف؛ تم استيراده باسم مؤقت مبني على الكود ويحتاج إلى مراجعة.")
	return "صنف دوائي #" + identifier, "", true
}

// readPrices fills the three price fields, rejecting the row only when a price
// is present but unusable. A missing price is normal — plenty of master
// catalogue rows are priced per supplier later — and must not lose the product.
func (c rowCursor) readPrices(prod *Product) bool {
	// Whichever price column the file carries is the one that must be readable.
	//
	// catalog.products has one price. A file heading its column "سعر البيع"
	// binds to price and one heading it "السعر" binds to the public price —
	// the same column, named two ways, and both fill the same field. Validating
	// only the first meant that a file of the second kind had its negative and
	// unparseable prices merely warned about and then imported as zero, because
	// the fallback below ran on a value nothing had checked.
	primary := FieldPublicPrice
	if !c.plan.Has(FieldPublicPrice) && c.plan.Has(FieldPrice) {
		primary = FieldPrice
	}

	price, ok := c.readAmount(FieldPrice, primary == FieldPrice)
	if !ok {
		return false
	}
	public, ok := c.readAmount(FieldPublicPrice, primary == FieldPublicPrice)
	if !ok {
		return false
	}

	prod.Price = price
	prod.OldPrice = public
	// Public price is the master catalog's official retail price. When only public price
	// is given, use it for both price and old_price.
	if prod.Price.IsZero() && prod.OldPrice.IsPositive() {
		prod.Price = prod.OldPrice
	} else if prod.OldPrice.IsZero() && prod.Price.IsPositive() {
		prod.OldPrice = prod.Price
	}

	c.readDiscount(prod)
	return true
}

// readAmount reads one money column. A required column that holds an unreadable
// value rejects the row; an optional one only warns.
func (c rowCursor) readAmount(field string, required bool) (money.Amount, bool) {
	raw := c.value(field)
	if raw == "" {
		return money.Zero, true
	}

	amt, info, err := CoerceMoney(raw)
	switch {
	case err == ErrNoValue:
		return money.Zero, true

	case err != nil:
		if required {
			c.reject(c.label(field), raw,
				fmt.Sprintf("تم رفض الصف: قيمة السعر «%s» ليست رقماً صالحاً.", raw))
			return money.Zero, false
		}
		c.warn(c.label(field), raw, fmt.Sprintf("تعذر قراءة القيمة «%s» كرقم؛ تم تجاهلها.", raw))
		return money.Zero, true

	case info.Percent:
		// A price column that says "25%" is a misaligned sheet, not a product
		// priced at twenty-five piastres. The parser knows the cell was a
		// percentage; staying silent here is how a whole column of prices
		// lands as small change with nothing in the report to explain it.
		if required {
			c.reject(c.label(field), raw,
				fmt.Sprintf("تم رفض الصف: قيمة السعر «%s» مكتوبة كنسبة مئوية وليست سعراً. يرجى تصحيح العمود في الملف.", raw))
			return money.Zero, false
		}
		c.warn(c.label(field), raw, "قيمة مكتوبة كنسبة مئوية في عمود سعر؛ تم تجاهلها.")
		return money.Zero, true

	case amt.IsNegative():
		c.reject(c.label(field), raw, "تم رفض الصف: السعر لا يمكن أن يكون قيمة سالبة.")
		return money.Zero, false

	case amt.Minor() > maxStorablePriceMinor:
		// Caught here rather than at the write, because NUMERIC(12,2) would
		// refuse it and abort the whole transaction for one bad cell — losing an
		// otherwise clean import of nine thousand rows.
		c.reject(c.label(field), raw,
			"تم رفض الصف: قيمة السعر تتجاوز الحد الأقصى المسموح به (9,999,999,999.99).")
		return money.Zero, false
	}

	if info.Rounded {
		c.warn(c.label(field), raw,
			fmt.Sprintf("تم تقريب القيمة إلى منزلتين عشريتين (%s).", amt.String()))
	}
	return amt, true
}

// readDiscount interprets the discount column, which arrives either as a
// percentage ("20%") or as an amount ("9.20"). Both are common in the same
// supplier's files, so the unit is decided per cell and not per column.
func (c rowCursor) readDiscount(prod *Product) {
	raw := c.value(FieldDiscount)
	if raw == "" {
		return
	}

	amt, info, err := CoerceMoney(raw)
	switch {
	case err == ErrNoValue:
		return
	case err != nil:
		c.warn(c.label(FieldDiscount), raw, "تعذر قراءة قيمة الخصم؛ تم تجاهلها.")
		return
	case amt.IsNegative():
		c.warn(c.label(FieldDiscount), raw, "قيمة الخصم سالبة؛ تم تجاهلها.")
		return
	}

	if info.Percent {
		if prod.Price.IsPositive() {
			// Percent of price, in minor units, truncated to the piastre. Both
			// operands are already scaled by 100, hence the 100*100 divisor.
			prod.Discount = money.FromMinor(prod.Price.Minor() * amt.Minor() / (100 * 100))
		}
		return
	}

	if prod.Price.IsPositive() && amt.Minor() >= prod.Price.Minor() {
		c.warn(c.label(FieldDiscount), raw, "قيمة الخصم تساوي السعر أو تزيد عليه؛ تم تجاهلها.")
		return
	}
	prod.Discount = amt
}
