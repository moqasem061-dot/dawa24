package pages

import (
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"strings"

	"github.com/a-h/templ"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

// View model for step two of the catalogue import: the column review.
//
// This step did not exist. The upload screen posted the file and immediately
// started processing it, so the first thing an admin saw was a finished run
// against a mapping nobody had looked at — and when that mapping was wrong, a
// review screen reporting nothing found and no way to tell why. The screen
// below is the missing half: what the file looks like, which column the
// importer thinks is which field, what those columns actually contain, and what
// the first two dozen products come out as. Nothing is staged until the admin
// presses the button at the bottom.

// ImportColumnOption is one column of the file, offered in a field's chooser.
type ImportColumnOption struct {
	// Value is the one-based column number submitted by the form.
	Value int
	// Label is what the admin reads: i18n.TDefault("w4m_ui.b_1").
	Label string
	// Sample is the first value found in that column.
	Sample string
}

// ImportFieldRow is one product field and its current binding.
type ImportFieldRow struct {
	Field      string
	Label      string
	Column     int
	Confidence string
	Header     string
	Sample     string
	// Critical marks a field an import really wants, so the screen can flag it
	// when it is unbound rather than letting it pass unnoticed.
	Critical bool
}

// Bound reports whether the field reads from a column.
func (f ImportFieldRow) Bound() bool { return f.Column > 0 }

// ImportPreviewColumn is one column of the preview table.
type ImportPreviewColumn struct {
	Field string
	Label string
}

// ImportPreviewRow is one previewed product.
type ImportPreviewRow struct {
	SourceRow int
	Values    []string
}

// ImportMappingView is the whole column-review screen.
type ImportMappingView struct {
	Session   *catalog.ImportSession
	Structure catalog.FileStructure

	// Fields is every mappable product field with its current column.
	Fields []ImportFieldRow
	// Options is the chooser offered for every field: the file's columns.
	Options []ImportColumnOption
	// Missing names the critical fields this mapping does not bind.
	Missing []string

	// PreviewColumns and PreviewRows are the sample of parsed products.
	PreviewColumns []ImportPreviewColumn
	PreviewRows    []ImportPreviewRow

	// FileHeaders and FileSampleRows are the spreadsheet as it was read, before
	// any mapping is applied — the same raw preview every other import wizard
	// shows. The mapped preview below it answers "what will this become"; this
	// one answers the question that has to come first, "did I upload the right
	// file, and did the reader find the right header row".
	FileHeaders    []string
	FileSampleRows [][]string
	// Issues are the findings from the previewed rows.
	Issues []catalog.RowIssue

	TotalProducts int
	RejectedRows  int
	DuplicateRows int

	Modes       []catalog.ImportModeOption
	Toggles     []ImportToggle
	Categories  []catalog.TaxonomyOption
	AIAvailable bool

	Notice     string
	NoticeKind string
}

// criticalFields are the ones the screen highlights when unbound.
var criticalFields = map[string]bool{
	catalog.FieldNameAR:      true,
	catalog.FieldSKU:         true,
	catalog.FieldPublicPrice: true,
}

// CoreFields returns the essential product identification fields.
func (v ImportMappingView) CoreFields() []ImportFieldRow {
	var out []ImportFieldRow
	coreSet := map[string]bool{
		catalog.FieldNameAR:      true,
		catalog.FieldSKU:         true,
		catalog.FieldBarcode:     true,
		catalog.FieldPublicPrice: true,
	}
	for _, f := range v.Fields {
		if coreSet[f.Field] {
			out = append(out, f)
		}
	}
	return out
}

// PharmaFields returns pharmaceutical, active ingredient and specification fields.
func (v ImportMappingView) PharmaFields() []ImportFieldRow {
	var out []ImportFieldRow
	pharmaSet := map[string]bool{
		catalog.FieldNameEN:        true,
		catalog.FieldGenericName:   true,
		catalog.FieldActive:        true,
		catalog.FieldDosageForm:    true,
		catalog.FieldConcentration: true,
		catalog.FieldUnit:          true,
		catalog.FieldManufacturer:  true,
	}
	for _, f := range v.Fields {
		if pharmaSet[f.Field] {
			out = append(out, f)
		}
	}
	return out
}

// PricingTaxonomyFields returns extra taxonomy and description fields.
func (v ImportMappingView) PricingTaxonomyFields() []ImportFieldRow {
	var out []ImportFieldRow
	pricingSet := map[string]bool{
		catalog.FieldPrice:         true,
		catalog.FieldCategory:      true,
		catalog.FieldDescriptionAR: true,
		catalog.FieldDescriptionEN: true,
		catalog.FieldStatus:        true,
	}
	for _, f := range v.Fields {
		if pricingSet[f.Field] {
			out = append(out, f)
		}
	}
	return out
}

// NewImportMappingView assembles the column-review screen from a preview run.
func NewImportMappingView(
	session *catalog.ImportSession, preview *catalog.ImportPreview,
	categories []catalog.TaxonomyOption, aiAvailable bool,
) ImportMappingView {
	view := ImportMappingView{
		Session:     session,
		Modes:       catalog.ImportModeOptions,
		Toggles:     importToggles(session.Options, aiAvailable),
		Categories:  categories,
		AIAvailable: aiAvailable,
	}
	if preview != nil {
		view.Structure = preview.Structure
		view.TotalProducts = preview.TotalProducts
		view.RejectedRows = preview.RejectedRows
		view.DuplicateRows = preview.DuplicateRows
		view.Issues = preview.Issues
	} else {
		view.Structure = session.Structure
	}

	view.Options = importColumnOptions(view.Structure)
	view.Fields = importFieldRows(view.Structure)
	view.Missing = view.Structure.MissingCritical()
	if preview != nil {
		view.PreviewColumns, view.PreviewRows = importPreviewTable(view.Fields, preview)
	}
	view.FileHeaders, view.FileSampleRows = rawFilePreview(view.Structure)
	return view
}

// rawFilePreview returns the head of the uploaded sheet as it was read.
//
// It used to assemble the table by transposing FileColumn.Samples —
// row[c] = col.Samples[r] — and Samples holds only the distinct, non-empty,
// truncated values of a column. Columns skip different cells, so index r of one
// column and index r of another were cells from different sheet rows: the
// preview showed a grid of values that never appeared together in the file,
// which is precisely the mis-reading it exists to reveal. It also took the
// minimum sample depth across columns, so one blank column erased the preview.
//
// FileStructure.Preview is the real thing, captured at analysis time.
func rawFilePreview(structure catalog.FileStructure) ([]string, [][]string) {
	if len(structure.Preview) == 0 {
		return nil, nil
	}

	width := structure.Width
	if width <= 0 {
		width = len(structure.Columns)
	}
	headers := make([]string, width)
	for i, col := range structure.Columns {
		if i >= width {
			break
		}
		headers[i] = col.Header
	}

	rows := structure.Preview
	if structure.HeaderRow > 0 && len(rows) > 0 {
		rows = rows[1:]
	}
	if len(rows) > importRawPreviewRows {
		rows = rows[:importRawPreviewRows]
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return headers, rows
}

// importRawPreviewRows caps the raw preview: enough to spot a shifted column,
// short enough not to become the page.
const importRawPreviewRows = 5

// importColumnOptions turns the file's columns into a chooser, labelled with
// whatever identifies them best: the header the file gave, or a sample value
// when it gave none.
func importColumnOptions(structure catalog.FileStructure) []ImportColumnOption {
	out := make([]ImportColumnOption, 0, len(structure.Columns))
	for _, col := range structure.Columns {
		label := col.Letter
		switch {
		case col.Header != "":
			label += " · " + col.Header
		case len(col.Samples) > 0:
			label += " · " + col.Samples[0]
		}
		out = append(out, ImportColumnOption{
			Value: col.Index, Label: label, Sample: col.SampleText(),
		})
	}
	return out
}

// importFieldRows lists every mappable field with its binding and evidence.
func importFieldRows(structure catalog.FileStructure) []ImportFieldRow {
	samples := make(map[int]string, len(structure.Columns))
	for _, col := range structure.Columns {
		samples[col.Index] = col.SampleText()
	}

	out := make([]ImportFieldRow, 0, len(structure.Fields))
	for _, binding := range structure.Fields {
		out = append(out, ImportFieldRow{
			Field:      binding.Field,
			Label:      binding.Label,
			Column:     binding.Column,
			Confidence: binding.Confidence,
			Header:     binding.Header,
			Sample:     samples[binding.Column],
			Critical:   criticalFields[binding.Field],
		})
	}
	return out
}

// importPreviewTable renders the sampled products as a table of the fields the
// mapping actually binds. Showing every field would be twenty mostly-empty
// columns; showing the bound ones is the check the admin came here to make.
func importPreviewTable(
	fields []ImportFieldRow, preview *catalog.ImportPreview,
) ([]ImportPreviewColumn, []ImportPreviewRow) {
	var columns []ImportPreviewColumn
	for _, field := range fields {
		if field.Bound() {
			columns = append(columns, ImportPreviewColumn{Field: field.Field, Label: field.Label})
		}
	}
	if len(columns) == 0 {
		return nil, nil
	}

	rows := make([]ImportPreviewRow, 0, len(preview.Products))
	for i, product := range preview.Products {
		row := ImportPreviewRow{Values: make([]string, 0, len(columns))}
		if i < len(preview.SourceRows) {
			row.SourceRow = preview.SourceRows[i]
		}
		for _, column := range columns {
			row.Values = append(row.Values, catalog.ProductFieldValue(product, column.Field))
		}
		rows = append(rows, row)
	}
	return columns, rows
}

// HeaderSummary describes the shape of the file in one line.
func (v ImportMappingView) HeaderSummary() string {
	parts := []string{v.Session.Filename}
	switch v.Structure.Format {
	case "xlsx":
		if v.Structure.Sheet != "" {
			parts = append(parts, fmt.Sprintf(i18n.TDefault("w4_ui.s_32"), v.Structure.Sheet))
		}
	case "csv":
		parts = append(parts, fmt.Sprintf(i18n.TDefault("w4_ui.s_33"), delimiterLabel(v.Structure.Delimiter)))
	}
	parts = append(parts, fmt.Sprintf(i18n.TDefault("w4_ui.s_182"), FormatCount(v.Structure.TotalRows)))
	parts = append(parts, fmt.Sprintf(i18n.TDefault("w4_ui.d_183"), v.Structure.Width))
	if v.Structure.BlockCount > 1 {
		parts = append(parts, fmt.Sprintf(i18n.TDefault("w4_ui.s_184"), FormatCount(v.Structure.BlockCount)))
	}
	return strings.Join(parts, " · ")
}

// HeaderRowLabel says which row the column titles were found on, or that none
// were, which is the single most important thing on this screen: a file read
// positionally is almost always a file read wrongly.
func (v ImportMappingView) HeaderRowLabel() string {
	if v.Structure.Positional || v.Structure.HeaderRow <= 0 {
		return i18n.TDefault("w4_ui.s_185_185")
	}
	return fmt.Sprintf(i18n.TDefault("w4_ui.d_34"), v.Structure.HeaderRow)
}

// CanProcess reports whether the mapping is usable enough to run.
func (v ImportMappingView) CanProcess() bool {
	return v.Session != nil && v.Session.IsReviewable() && len(v.Missing) == 0
}

// PreviewIsEmpty reports whether the current mapping yields no products at all,
// which the screen states plainly instead of drawing an empty table.
func (v ImportMappingView) PreviewIsEmpty() bool { return v.TotalProducts == 0 }

// mappingAction builds a POST target on the session.
func mappingAction(view ImportMappingView, verb string) templ.SafeURL {
	return templ.SafeURL(fmt.Sprintf("/admin/products/import/%s/%s", view.Session.PublicID, verb))
}

// fieldSelectName is the form field one binding is submitted under. It matches
// what readLayoutOverrides reads.
func fieldSelectName(field string) string { return "col_" + field }

// tonedHeaderRow colours the header-row tile: a file read positionally is
// almost certainly a file read wrongly, and the tile says so.
func tonedHeaderRow(view ImportMappingView) string {
	if view.Structure.Positional || view.Structure.HeaderRow <= 0 {
		return "error"
	}
	return ""
}

// tonedProducts colours the yield tile. Zero products under the current mapping
// is the failure this whole screen exists to catch before a run.
func tonedProducts(view ImportMappingView) string {
	if view.TotalProducts == 0 {
		return "error"
	}
	return "add"
}

// joinList renders a list of names as Arabic prose.
func joinList(items []string) string { return strings.Join(items, i18n.TDefault("w4_ui.w4str_35_35")) }
