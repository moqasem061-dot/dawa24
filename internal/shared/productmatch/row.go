package productmatch

import (
	"fmt"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Row reading.
//
// Every decision made here is recorded. A row that is dropped says why, a value
// that was assumed says it was assumed, and a price that could not be read is a
// finding the vendor sees rather than a zero written into their catalogue.
//
// The alternative — the one the previous importer took — is to parse
// permissively, discard what does not fit, and report success. That importer
// silently dropped every price it could not parse and told the vendor their
// nine thousand products had imported cleanly.

// Issue is one finding about one row, addressed the way the vendor sees their
// file: by spreadsheet row number and column heading.
type Issue struct {
	Row      int      `json:"row"`
	Field    Field    `json:"field,omitempty"`
	Column   string   `json:"column,omitempty"`
	Value    string   `json:"value,omitempty"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

// Row is one product line read out of the sheet.
type Row struct {
	// Number is the spreadsheet row number as the vendor sees it in Excel's
	// gutter, one-based.
	Number int `json:"number"`

	Name          string `json:"name,omitempty"`
	NameEN        string `json:"name_en,omitempty"`
	Scientific    string `json:"scientific,omitempty"`
	SKU           string `json:"sku,omitempty"`
	Barcode       string `json:"barcode,omitempty"`
	Manufacturer  string `json:"manufacturer,omitempty"`
	DosageForm    string `json:"dosage_form,omitempty"`
	Concentration string `json:"concentration,omitempty"`
	Unit          string `json:"unit,omitempty"`
	PackSize      int    `json:"pack_size,omitempty"`
	Category      string `json:"category,omitempty"`

	// PublicPrice is the printed retail price, the base the discount applies to.
	PublicPrice money.Amount `json:"public_price"`
	// NetPrice is what the pharmacy pays, either stated or derived.
	NetPrice money.Amount `json:"net_price"`
	// CostPrice is the vendor's own cost, never shown to buyers.
	CostPrice money.Amount `json:"cost_price"`
	// CostDiscountBps is the vendor's cost/purchase discount in basis points (e.g. 25.0% is 2500).
	CostDiscountBps int64 `json:"cost_discount_bps"`
	// DiscountBps is the discount in hundredths of a percent, so 32.5% is 3250
	// and stays exact. A percentage held as a float is how a catalogue ends up
	// priced a piastre out on every line.
	DiscountBps int64  `json:"discount_bps"`
	Bonus       string `json:"bonus,omitempty"`
	// PricingNote explains how the three prices were reconciled, when they were
	// not all stated.
	PricingNote string `json:"pricing_note,omitempty"`

	Quantity     int  `json:"quantity"`
	HasQuantity  bool `json:"has_quantity"`
	MinOrderQty  int  `json:"min_order_qty"`
	MinThreshold int  `json:"min_threshold"`

	BatchNumber string     `json:"batch_number,omitempty"`
	ExpiryDate  *time.Time `json:"expiry_date,omitempty"`
	Warehouse   string     `json:"warehouse,omitempty"`
	Branch      string     `json:"branch,omitempty"`
	Negotiable  *bool      `json:"negotiable,omitempty"`
	Status      string     `json:"status,omitempty"`
	Image       string     `json:"image,omitempty"`
	Notes       string     `json:"notes,omitempty"`

	Issues []Issue `json:"issues,omitempty"`
}

// Identity reports whether the row carries anything a product can be matched by.
func (r *Row) Identity() bool {
	return r.Name != "" || r.SKU != "" || r.Barcode != ""
}

// HasErrors reports whether the row carries a blocking finding.
func (r *Row) HasErrors() bool {
	for _, i := range r.Issues {
		if i.Severity == SeverityError {
			return true
		}
	}
	return false
}

// DisplayName is what the review table shows for the row.
func (r *Row) DisplayName() string {
	switch {
	case r.Name != "":
		return r.Name
	case r.NameEN != "":
		return r.NameEN
	case r.SKU != "":
		return "صنف بالكود " + r.SKU
	case r.Barcode != "":
		return "صنف بالباركود " + r.Barcode
	}
	return fmt.Sprintf("صف %d", r.Number)
}

// ParseOptions are the reading rules the vendor chose in the settings step.
type ParseOptions struct {
	// DefaultMinOrderQty applies where the file states none.
	DefaultMinOrderQty int
	// DefaultMinThreshold applies where the file states none.
	DefaultMinThreshold int
	// BlankQuantityIsZero treats a missing quantity as "none in stock" rather
	// than "unknown, leave the current balance alone". The distinction matters:
	// a supplier sending a full catalogue means zero, a supplier sending an
	// update of twelve lines means unknown.
	BlankQuantityIsZero bool
	// InferDosageForm fills the pharmaceutical form from the product name where
	// the file has no column for it.
	InferDosageForm bool
	// InferConcentration reads the strength out of the product name the same
	// way. It is not a guess — the value is quoted from the name verbatim.
	InferConcentration bool
	// RejectExpired refuses rows whose expiry date has already passed rather
	// than importing stock that cannot legally be sold.
	RejectExpired bool
	// Now is the clock, injected so tests are not calendar-dependent.
	Now time.Time
}

// DefaultParseOptions are the safe settings the wizard starts on.
func DefaultParseOptions() ParseOptions {
	return ParseOptions{
		DefaultMinOrderQty:  1,
		DefaultMinThreshold: 0,
		InferDosageForm:     true,
		InferConcentration:  true,
		RejectExpired:       true,
	}
}

// Reader turns spreadsheet rows into products under one resolved mapping.
//
// It holds no per-row state, so a caller may run several in parallel over
// different slices of a very large file.
type Reader struct {
	cols    map[Field]int
	headers []string
	opts    ParseOptions
	// codeShape and nameShape record what the file's identity columns look like
	// overall, which is what lets a single row whose code and name are swapped
	// be recognised as swapped rather than as two bad values.
	codeIsNumeric bool
	nameIsWordy   bool
}

// NewReader prepares a reader for one mapping.
func NewReader(m *Mapping, opts ParseOptions) *Reader {
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	r := &Reader{cols: map[Field]int{}, opts: opts}
	if m == nil {
		return r
	}
	for f, col := range m.ByField {
		r.cols[f] = col
	}
	r.headers = make([]string, len(m.Columns))
	for i, c := range m.Columns {
		r.headers[i] = c.Header
	}
	if col, ok := r.cols[FieldSKU]; ok && col < len(m.Columns) {
		if p := m.Columns[col].Profile; p != nil {
			r.codeIsNumeric = p.Rate(p.Numeric) >= 0.7
		}
	}
	if col, ok := r.cols[FieldName]; ok && col < len(m.Columns) {
		if p := m.Columns[col].Profile; p != nil {
			r.nameIsWordy = p.Rate(p.Wordy) >= 0.5
		}
	}
	return r
}

// Rebind swaps in a new mapping, for a sheet whose second section has its own
// header row.
func (r *Reader) Rebind(m *Mapping) {
	next := NewReader(m, r.opts)
	r.cols, r.headers = next.cols, next.headers
	r.codeIsNumeric, r.nameIsWordy = next.codeIsNumeric, next.nameIsWordy
}

// cell returns a field's value for this row, or empty when unmapped or short.
func (r *Reader) cell(row []string, f Field) string {
	idx, ok := r.cols[f]
	if !ok || idx < 0 || idx >= len(row) {
		return ""
	}
	return sheet.CleanCell(row[idx])
}

// header names a field the way the vendor's own file names it.
func (r *Reader) header(f Field) string {
	if idx, ok := r.cols[f]; ok && idx >= 0 && idx < len(r.headers) && r.headers[idx] != "" {
		return r.headers[idx]
	}
	return f.Label()
}

// has reports whether a field is mapped at all.
func (r *Reader) has(f Field) bool {
	_, ok := r.cols[f]
	return ok
}

// Read interprets one spreadsheet row.
//
// It returns the row even when findings were raised; only a row with no usable
// identity at all is refused outright, because a row with a bad price is still
// a product the vendor stocks and dropping it loses more than it protects.
func (r *Reader) Read(number int, cells []string) (*Row, bool) {
	out := &Row{
		Number:       number,
		MinOrderQty:  r.opts.DefaultMinOrderQty,
		MinThreshold: r.opts.DefaultMinThreshold,
	}

	r.readIdentity(out, cells)
	if !out.Identity() {
		out.add(Issue{
			Row: number, Severity: SeverityError,
			Message: "تم تجاهل الصف: لا يحتوي على اسم صنف ولا كود ولا باركود.",
		})
		return out, false
	}

	r.readAttributes(out, cells)
	r.readPricing(out, cells)
	r.readStock(out, cells)
	r.readFlags(out, cells)
	return out, !out.HasErrors()
}

// readIdentity fills the matching fields and repairs a swapped row.
func (r *Reader) readIdentity(out *Row, cells []string) {
	out.Name = r.cell(cells, FieldName)
	out.NameEN = r.cell(cells, FieldNameEN)
	out.SKU = r.cell(cells, FieldSKU)
	out.Barcode = normalizeBarcode(r.cell(cells, FieldBarcode))
	out.Scientific = r.cell(cells, FieldScientific)
	out.Manufacturer = r.cell(cells, FieldManufacturer)

	r.repairSwappedIdentity(out)

	if out.Name == "" && out.NameEN != "" {
		out.Name = out.NameEN
	}
	if out.Barcode != "" && !sheet.ValidGTIN(out.Barcode) && len(out.Barcode) >= 12 {
		out.add(Issue{
			Row: out.Number, Field: FieldBarcode, Column: r.header(FieldBarcode), Value: out.Barcode,
			Severity: SeverityWarning,
			Message:  "الباركود بطول صحيح لكن رقم التحقق غير مطابق؛ قد يكون به خطأ إملائي.",
		})
	}
}

// repairSwappedIdentity fixes the row whose code and name are in each other's
// columns.
//
// This is not hypothetical. A real distributor export of 1,417 rows carries the
// product name in the item-code column and the code in the name column on
// perhaps one row in eight — an operator pasted a block shifted by one. Without
// this the file imports a hundred and seventy products named "23169" and a
// hundred and seventy codes reading "هاى بيوتك ان شراب 600 مجم".
func (r *Reader) repairSwappedIdentity(out *Row) {
	if !r.has(FieldName) || !r.has(FieldSKU) {
		return
	}
	if out.Name == "" || out.SKU == "" {
		return
	}
	// Only worth suspecting when the columns are consistently the other shape.
	if !r.codeIsNumeric || !r.nameIsWordy {
		return
	}
	if !looksCode(out.Name) || !isWordy(out.SKU) {
		return
	}
	out.Name, out.SKU = out.SKU, out.Name
	out.add(Issue{
		Row: out.Number, Field: FieldName, Column: r.header(FieldName), Value: out.SKU,
		Severity: SeverityWarning,
		Message: "الاسم والكود في هذا الصف كانا متبادلين بين العمودين؛ تم تصحيحهما تلقائياً " +
			"بناءً على شكل القيم.",
	})
}

// isWordy reports whether a value has the shape of a name rather than a code.
func isWordy(v string) bool {
	if len(strings.Fields(v)) < 2 {
		return false
	}
	return sheet.Profile(v).Letters() >= 6
}

// normalizeBarcode reduces a barcode cell to its digits, which is how a value
// typed with spaces or exported with a leading apostrophe is recovered.
func normalizeBarcode(v string) string {
	if v == "" {
		return ""
	}
	digits := sheet.DigitsOnly(v)
	if digits == "" || len(digits) < 6 {
		return sheet.CleanCell(v)
	}
	return digits
}

// readAttributes fills the descriptive fields, inferring the two the vendor
// asked to have inferred.
func (r *Reader) readAttributes(out *Row, cells []string) {
	out.DosageForm = r.cell(cells, FieldDosageForm)
	out.Concentration = sheet.NormalizeDigits(r.cell(cells, FieldConcentration))
	out.Unit = r.cell(cells, FieldUnit)
	out.Category = r.cell(cells, FieldCategory)
	out.Image = r.cell(cells, FieldImage)
	out.Notes = r.cell(cells, FieldNotes)
	out.Bonus = r.cell(cells, FieldBonus)

	if n := r.cell(cells, FieldPackSize); n != "" {
		if v, err := sheet.CoerceInt(n); err == nil && v > 0 {
			out.PackSize = int(v)
		}
	}

	full := out.Name + " " + out.NameEN
	if out.DosageForm == "" && r.opts.InferDosageForm {
		out.DosageForm = InferDosageForm(full)
	}
	if out.Concentration == "" && r.opts.InferConcentration {
		out.Concentration = InferConcentration(full)
	}
}

// add records a finding, capping how many one row may carry so a single
// pathological line cannot fill the report.
func (r *Row) add(issue Issue) {
	const perRowCap = 8
	if len(r.Issues) >= perRowCap {
		return
	}
	r.Issues = append(r.Issues, issue)
}
