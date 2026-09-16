package catalog

import "sort"

// SheetBlock is one contiguous run of data rows sharing a single header.
type SheetBlock struct {
	// Index is the block's position in the sheet, from zero.
	Index int
	// HeaderRow is the zero-based row holding this block's column titles, or -1
	// when the block is read positionally.
	HeaderRow int
	// FirstRow and LastRow bound the data rows, inclusive and zero-based.
	FirstRow int
	LastRow  int
	// Plan is how this block's columns map to product fields.
	Plan ColumnPlan
}

// RowCount is how many sheet rows the block spans, blank ones included.
func (b SheetBlock) RowCount() int {
	if b.LastRow < b.FirstRow {
		return 0
	}
	return b.LastRow - b.FirstRow + 1
}

// SheetLayout is the structure found in one worksheet.
type SheetLayout struct {
	Blocks []SheetBlock
	// HeaderRows lists every row identified as column titles, the first of which
	// is the primary header.
	HeaderRows []int
	// Primary is the dominant block's column plan — what the admin sees and
	// overrides in the wizard.
	Primary ColumnPlan
	// Positional is true when no header was found anywhere and columns are read
	// by position. The admin is warned prominently in that case.
	Positional bool
	// TotalRows is the worksheet's row count.
	TotalRows int
	// DataRows is how many rows fall inside a block, before blank and duplicate
	// filtering. It is the number the admin should expect to see accounted for.
	DataRows int
	// VariantBlocks counts blocks whose column plan differs from the primary.
	// A non-zero value means the sheet stacks differently-shaped sections.
	VariantBlocks int
}

// LayoutOverrides are the admin's corrections to the detected structure,
// applied before any row is read.
//
// Every field is optional. The wizard sends only what the admin actually
// changed, so an untouched analysis round-trips unmodified.
type LayoutOverrides struct {
	// HeaderRow forces which row holds the column titles, one-based as the admin
	// sees it in Excel. Zero means "keep what was detected".
	HeaderRow int `json:"header_row,omitempty"`
	// FirstDataRow and LastDataRow bound the rows to read, one-based and
	// inclusive. Zero means "no bound".
	FirstDataRow int `json:"first_data_row,omitempty"`
	LastDataRow  int `json:"last_data_row,omitempty"`
	// Columns maps a field name to a one-based column number. A value of
	// IgnoreColumn unbinds the field so its column is not read at all.
	Columns map[string]int `json:"columns,omitempty"`
	// Sheet forces a worksheet by name, for a workbook whose largest tab is not
	// the one the admin wants.
	Sheet string `json:"sheet,omitempty"`
}

// IgnoreColumn unbinds a field in LayoutOverrides.Columns. It is negative so it
// cannot collide with a real one-based column number.
const IgnoreColumn = -1

// IsZero reports whether the admin changed nothing.
func (o LayoutOverrides) IsZero() bool {
	return o.HeaderRow == 0 && o.FirstDataRow == 0 && o.LastDataRow == 0 &&
		len(o.Columns) == 0 && o.Sheet == ""
}

// minHeaderScore is the score a row must reach to be treated as a section
// header on its own evidence rather than by resembling the primary header.
//
// One confident binding scores 100, so this demands roughly two — enough that a
// data row containing the word i18n.TDefault("w4_ui.s_33_33") in a product name cannot split the sheet.
const minHeaderScore = 150

// AnalyzeLayout finds the header rows and data blocks in a worksheet.
func AnalyzeLayout(data *SheetData) SheetLayout {
	layout := SheetLayout{Positional: true}
	if data == nil || len(data.Rows) == 0 {
		return layout
	}
	layout.TotalRows = len(data.Rows)

	primaryIdx := DetectHeaderRow(data.Rows)
	if primaryIdx < 0 {
		layout.Blocks = []SheetBlock{{
			Index:     0,
			HeaderRow: -1,
			FirstRow:  0,
			LastRow:   len(data.Rows) - 1,
			Plan:      positionalPlan(data.Width),
		}}
		layout.Primary = layout.Blocks[0].Plan
		layout.DataRows = len(data.Rows)
		return layout
	}

	layout.Positional = false
	layout.HeaderRows = findHeaderRows(data.Rows, primaryIdx)
	layout.Blocks = buildBlocks(data.Rows, layout.HeaderRows)

	if len(layout.Blocks) > 0 {
		layout.Primary = layout.Blocks[0].Plan
	}
	for _, b := range layout.Blocks {
		layout.DataRows += b.RowCount()
		if b.Index > 0 && !samePlan(b.Plan, layout.Primary) {
			layout.VariantBlocks++
		}
	}
	return layout
}

// positionalPlan is the fallback for a sheet with no header anywhere: the layout
// every Egyptian distributor export shares, narrowed for a single-column list.
func positionalPlan(width int) ColumnPlan {
	if width <= 1 {
		return ColumnPlan{Columns: map[string]int{FieldNameAR: 0}, Positional: true}
	}
	return ColumnPlan{
		Columns:    map[string]int{FieldSKU: 0, FieldNameAR: 1, FieldManufacturer: 2},
		Positional: true,
	}
}

// findHeaderRows locates every header in the sheet, starting from the primary.
//
// Two things count as a header. A row that echoes the primary header's labels is
// a reprint from a paginated export. A row that scores strongly as a header in
// its own right, with labels the primary does not have, starts a new section
// with a different shape. Everything else is data.
//
// The cheap resemblance test runs first on every row; the expensive scoring runs
// only on the few rows that survive a coarse filter, which keeps this linear
// over a nine-thousand-row sheet.
func findHeaderRows(records [][]string, primaryIdx int) []int {
	primaryKeys := normalizedKeySet(records[primaryIdx])
	headers := []int{primaryIdx}

	for i := primaryIdx + 1; i < len(records); i++ {
		row := records[i]
		if isRepeatedHeader(row, primaryKeys) {
			headers = append(headers, i)
			continue
		}
		if !couldBeSectionHeader(row) {
			continue
		}
		if scoreHeaderCandidate(row) >= minHeaderScore {
			headers = append(headers, i)
		}
	}

	sort.Ints(headers)
	return headers
}

// couldBeSectionHeader is the coarse filter in front of the expensive scoring.
//
// Column titles are words, not values: a row carrying any purely numeric cell is
// data, and a row with fewer than three labels is too thin to re-shape a
// section. Both tests are a few string scans, against a full column-plan
// evaluation for the rows that pass.
func couldBeSectionHeader(row []string) bool {
	filled := 0
	for _, cell := range row {
		clean := CleanCellString(cell)
		if clean == "" {
			continue
		}
		if digitsOnlyPattern.MatchString(NormalizeDigits(clean)) {
			return false
		}
		filled++
	}
	return filled >= 3
}

// buildBlocks turns the header rows into the data ranges between them.
//
// A header with no rows before the next header — two title rows stacked, which
// happens when an export repeats the header and a subtitle — yields an empty
// block that is dropped rather than carried as a zero-row section.
func buildBlocks(records [][]string, headerRows []int) []SheetBlock {
	var blocks []SheetBlock

	for i, headerRow := range headerRows {
		first := headerRow + 1
		last := len(records) - 1
		if i+1 < len(headerRows) {
			last = headerRows[i+1] - 1
		}
		if first > last {
			continue
		}

		blocks = append(blocks, SheetBlock{
			Index:     len(blocks),
			HeaderRow: headerRow,
			FirstRow:  first,
			LastRow:   last,
			// The block's own rows, not the sheet's: a file that stacks two
			// differently shaped sections must have each section's columns
			// judged on the values that section actually holds.
			Plan: PlanColumns(records[headerRow], sampleRows(records, first, last)),
		})
	}
	return blocks
}

// sampleRowsLimit is how many data rows are measured to profile a block.
//
// Two hundred is far past the point where another row changes a column's
// profile, and it keeps layout analysis proportional to the number of blocks
// rather than to the size of the file.
const sampleRowsLimit = 200

// sampleRows returns the head of a block's data rows, for value profiling.
func sampleRows(records [][]string, first, last int) [][]string {
	if first < 0 || first >= len(records) {
		return nil
	}
	if last >= len(records) {
		last = len(records) - 1
	}
	if last-first+1 > sampleRowsLimit {
		last = first + sampleRowsLimit - 1
	}
	return records[first : last+1]
}

// samePlan reports whether two blocks bind the same fields to the same columns.
func samePlan(a, b ColumnPlan) bool {
	if len(a.Columns) != len(b.Columns) {
		return false
	}
	for field, col := range a.Columns {
		if other, ok := b.Columns[field]; !ok || other != col {
			return false
		}
	}
	return true
}

// Apply folds the admin's corrections into a detected layout.
//
// Overrides are absolute, not additive: a forced header row rebuilds the blocks
// from that row, and a forced column binding replaces whatever was detected for
// that field in every block. The admin's judgement wins over the analysis
// everywhere the two disagree — that is the entire point of the review step.
func (l SheetLayout) Apply(data *SheetData, o LayoutOverrides) SheetLayout {
	if data == nil || o.IsZero() {
		return l.applyColumnOverrides(o)
	}

	out := l
	if o.HeaderRow > 0 && o.HeaderRow <= len(data.Rows) {
		headerIdx := o.HeaderRow - 1
		out.HeaderRows = findHeaderRows(data.Rows, headerIdx)
		out.Blocks = buildBlocks(data.Rows, out.HeaderRows)
		out.Positional = false
		if len(out.Blocks) > 0 {
			out.Primary = out.Blocks[0].Plan
		}
	}

	out = out.applyRowBounds(o)
	out = out.applyColumnOverrides(o)

	out.DataRows, out.VariantBlocks = 0, 0
	for _, b := range out.Blocks {
		out.DataRows += b.RowCount()
		if b.Index > 0 && !samePlan(b.Plan, out.Primary) {
			out.VariantBlocks++
		}
	}
	return out
}

// applyRowBounds clips the blocks to the admin's chosen row range, dropping any
// block that falls entirely outside it.
func (l SheetLayout) applyRowBounds(o LayoutOverrides) SheetLayout {
	if o.FirstDataRow <= 0 && o.LastDataRow <= 0 {
		return l
	}

	first, last := 0, l.TotalRows-1
	if o.FirstDataRow > 0 {
		first = o.FirstDataRow - 1
	}
	if o.LastDataRow > 0 {
		last = o.LastDataRow - 1
	}

	out := l
	out.Blocks = nil
	for _, b := range l.Blocks {
		b.FirstRow = max(b.FirstRow, first)
		b.LastRow = min(b.LastRow, last)
		if b.FirstRow > b.LastRow {
			continue
		}
		b.Index = len(out.Blocks)
		out.Blocks = append(out.Blocks, b)
	}
	return out
}

// applyColumnOverrides rebinds fields across every block.
func (l SheetLayout) applyColumnOverrides(o LayoutOverrides) SheetLayout {
	if len(o.Columns) == 0 {
		return l
	}

	out := l
	out.Blocks = make([]SheetBlock, len(l.Blocks))
	copy(out.Blocks, l.Blocks)
	for i := range out.Blocks {
		out.Blocks[i].Plan = overridePlan(out.Blocks[i].Plan, o.Columns)
	}
	out.Primary = overridePlan(l.Primary, o.Columns)
	if len(out.Blocks) > 0 {
		out.Primary = out.Blocks[0].Plan
	}
	return out
}

// overridePlan returns a copy of plan with the admin's bindings applied.
func overridePlan(plan ColumnPlan, columns map[string]int) ColumnPlan {
	out := ColumnPlan{
		Columns:    make(map[string]int, len(plan.Columns)),
		Unmapped:   plan.Unmapped,
		Positional: plan.Positional,
	}
	for field, col := range plan.Columns {
		out.Columns[field] = col
	}

	for field, oneBased := range columns {
		if oneBased == IgnoreColumn {
			delete(out.Columns, field)
			continue
		}
		if oneBased > 0 {
			out.Columns[field] = oneBased - 1
		}
	}

	// Bindings drive the report and the per-field labels, so they are rebuilt
	// from the resolved columns rather than left describing the old mapping.
	for _, b := range plan.Bindings {
		if col, ok := out.Columns[b.Field]; ok && col == b.Index {
			out.Bindings = append(out.Bindings, b)
		}
	}
	for field, col := range out.Columns {
		if hasBinding(out.Bindings, field) {
			continue
		}
		out.Bindings = append(out.Bindings, ColumnBinding{
			Field: field,
			Label: FieldLabels[field],
			Index: col,
			// An admin's own binding is certain by definition; it is not a guess
			// the report should ask them to double-check.
			Score:  100,
			Header: FieldLabels[field],
		})
	}
	sort.Slice(out.Bindings, func(i, j int) bool { return out.Bindings[i].Index < out.Bindings[j].Index })
	return out
}

func hasBinding(bindings []ColumnBinding, field string) bool {
	for _, b := range bindings {
		if b.Field == field {
			return true
		}
	}
	return false
}
