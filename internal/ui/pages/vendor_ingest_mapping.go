package pages

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// The column-mapping step, read the way a vendor actually thinks about it.
//
// It used to be a table of the file's columns, each with a dropdown of the
// twenty-nine fields the platform knows. That reads backwards. A vendor does
// not open the screen wondering "what is column G?" — they open it wondering
// "did it find my price column?", and a file whose price column is missing
// altogether showed nothing at all, because a row only existed where a column
// did. The required field was invisible precisely when it mattered.
//
// So the table is now the system's fields, always all of them, each with a
// dropdown of the file's columns. A missing price column is a visible empty
// row marked إلزامي. The auto-resolved mapping fills the dropdowns in, so the
// common case is still "read it and continue".
//
// The fields offered are productmatch.VendorFields — the vendor's own variant
// and stock columns. Category, active ingredient and the description columns
// belong to the master catalogue, which no vendor import writes, and are not
// offered here at all.

// FieldMappingRow is one system field and whichever file column is bound to it.
type FieldMappingRow struct {
	Spec productmatch.Spec
	// ColumnIndex is the zero-based file column bound to this field, or -1.
	ColumnIndex int
	// Header is that column's heading, for the confirmation line.
	Header string
	// Confidence and Score are the resolver's own report on the binding.
	Confidence productmatch.Confidence
	Score      float64
	// Why is the resolver's evidence, shown so a surprising binding can be
	// argued with rather than merely overridden.
	Why []string
}

// Bound reports whether a column is assigned to this field.
func (r FieldMappingRow) Bound() bool { return r.ColumnIndex >= 0 }

// Required reports whether the import cannot proceed without this field.
func (r FieldMappingRow) Required() bool { return r.Spec.Need == productmatch.NeedCore }

// Important reports whether omitting the field changes what is written.
func (r FieldMappingRow) Important() bool { return r.Spec.Need == productmatch.NeedImportant }

// NeedLabel renders the field's necessity as the badge text.
func (r FieldMappingRow) NeedLabel() string {
	switch r.Spec.Need {
	case productmatch.NeedCore:
		return "إلزامي"
	case productmatch.NeedImportant:
		return "مهم"
	default:
		return "اختياري"
	}
}

// NeedTone maps necessity onto the badge palette.
func (r FieldMappingRow) NeedTone() string {
	switch r.Spec.Need {
	case productmatch.NeedCore:
		return "badge-rose"
	case productmatch.NeedImportant:
		return "badge-amber"
	default:
		return "badge-slate"
	}
}

// FormName is the input name this row submits under.
func (r FieldMappingRow) FormName() string { return "field_" + string(r.Spec.Field) }

// FileColumn is one column of the uploaded file, as offered in a dropdown.
type FileColumn struct {
	Index   int      `json:"index"`
	Header  string   `json:"header"`
	Preview []string `json:"preview"`
}

// Title renders the column the way the dropdown names it.
func (c FileColumn) Title() string {
	if c.Header != "" {
		return fmt.Sprintf("%s — العمود %d", c.Header, c.Index+1)
	}
	return fmt.Sprintf("العمود %d", c.Index+1)
}

// FileColumns lists the uploaded file's columns in sheet order.
func (v VendorImportView) FileColumns() []FileColumn {
	cols := v.MappedColumns()
	out := make([]FileColumn, 0, len(cols))
	for _, c := range cols {
		if c == nil {
			continue
		}
		out = append(out, FileColumn{Index: c.Index, Header: c.Header, Preview: c.Preview})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

// FileColumnsJSON is the same list as a JSON literal, so the dropdown can show
// the chosen column's sample values without a round trip.
func (v VendorImportView) FileColumnsJSON() string {
	b, err := json.Marshal(v.FileColumns())
	if err != nil {
		return "[]"
	}
	return string(b)
}

// FieldMappingRows lists every field a vendor import can read, required first,
// each carrying whatever column the resolver bound to it.
func (v VendorImportView) FieldMappingRows() []FieldMappingRow {
	bound := make(map[productmatch.Field]*productmatch.Column)
	for _, c := range v.MappedColumns() {
		if c == nil || c.Ignored || c.Field == "" {
			continue
		}
		// First binding wins. A field bound twice is a conflict the resolver
		// already reported; showing the earlier column keeps this table
		// agreeing with that message.
		if _, seen := bound[c.Field]; !seen {
			bound[c.Field] = c
		}
	}

	specs := productmatch.VendorFields.Specs()
	rows := make([]FieldMappingRow, 0, len(specs))
	for _, spec := range specs {
		row := FieldMappingRow{Spec: spec, ColumnIndex: -1}
		if c, ok := bound[spec.Field]; ok {
			row.ColumnIndex = c.Index
			row.Header = c.Header
			row.Confidence = c.Confidence
			row.Score = c.Score
			row.Why = c.Why
		}
		rows = append(rows, row)
	}

	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Spec.Need < rows[j].Spec.Need })
	return rows
}

// UnmappedRequired lists the required fields no column was bound to, so the
// step can refuse to continue with a reason rather than failing later.
func (v VendorImportView) UnmappedRequired() []FieldMappingRow {
	var out []FieldMappingRow
	for _, r := range v.FieldMappingRows() {
		if r.Required() && !r.Bound() {
			out = append(out, r)
		}
	}
	return out
}

// UnusedColumns lists the file's columns no field claims. They are skipped by
// the import, and saying so is cheaper than a vendor discovering it.
func (v VendorImportView) UnusedColumns() []FileColumn {
	used := make(map[int]bool)
	for _, r := range v.FieldMappingRows() {
		if r.Bound() {
			used[r.ColumnIndex] = true
		}
	}
	var out []FileColumn
	for _, c := range v.FileColumns() {
		if !used[c.Index] {
			out = append(out, c)
		}
	}
	return out
}

// SelectedValue renders the bound column index for the dropdown's initial
// state, or the empty string when nothing is bound.
func (r FieldMappingRow) SelectedValue() string {
	if r.ColumnIndex < 0 {
		return ""
	}
	return fmt.Sprint(r.ColumnIndex)
}

// fieldSampleText renders the bound column's sample values server-side, so the
// table reads correctly before Alpine has run and with scripting disabled.
func fieldSampleText(row FieldMappingRow, cols []FileColumn) string {
	if !row.Bound() {
		return ""
	}
	for _, c := range cols {
		if c.Index == row.ColumnIndex {
			return strings.Join(c.Preview, " · ")
		}
	}
	return ""
}

// vendorImportRawPreviewRows caps the raw file preview: enough rows to notice a
// shifted column, short enough not to become the page.
const vendorImportRawPreviewRows = 5

// RawFilePreview returns the head of the uploaded sheet exactly as it was read.
//
// It used to build the table by transposing each column's Preview slice:
// row[c] = cols[c].Preview[r]. Column.Preview comes from ColumnProfile.Sample,
// which only records values that are non-empty *and* not already seen. Columns
// therefore skip different cells, so index r of column A and index r of column
// B were cells from different spreadsheet rows and the table showed a grid of
// values that never appeared together in the file.
//
// It also took the *minimum* sample depth across columns, so one entirely blank
// column made depth zero and the preview vanished.
//
// Analysis.Preview is the real thing: raw rows from the header row down, each
// padded to Layout.Width by previewSlice. Row zero is the header row when one
// was detected, so it is dropped here rather than repeated under itself.
func (v VendorImportView) RawFilePreview() ([]string, [][]string) {
	if v.Analysis == nil || len(v.Analysis.Preview) == 0 {
		return nil, nil
	}

	headers := make([]string, 0, v.Analysis.Layout.Width)
	for i := 0; i < v.Analysis.Layout.Width; i++ {
		label := ""
		if i < len(v.Analysis.Layout.Headers) {
			label = strings.TrimSpace(v.Analysis.Layout.Headers[i])
		}
		headers = append(headers, label)
	}

	rows := v.Analysis.Preview
	if v.Analysis.Layout.HeaderRow >= 0 && len(rows) > 0 {
		rows = rows[1:]
	}
	if len(rows) > vendorImportRawPreviewRows {
		rows = rows[:vendorImportRawPreviewRows]
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return headers, rows
}
