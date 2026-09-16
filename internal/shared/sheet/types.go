package sheet

import "errors"

// Format is the container a file turned out to be.
type Format string

const (
	FormatXLSX Format = "xlsx"
	FormatXLS  Format = "xls"
	FormatCSV  Format = "csv"
	FormatHTML Format = "html"
	// FormatXML2003 is Microsoft Office XML Spreadsheet, which arrives named
	// .xls and is neither BIFF nor HTML. See reader_xml2003.go.
	FormatXML2003 Format = "xml2003"
)

// Label renders a format for the review screen.
func (f Format) Label() string {
	switch f {
	case FormatXLSX:
		return "Excel (.xlsx)"
	case FormatXLS:
		return "Excel 97-2003 (.xls)"
	case FormatHTML:
		return "جدول HTML"
	case FormatXML2003:
		return "Excel XML 2003"
	default:
		return "نص مفصول (CSV)"
	}
}

// SheetInfo describes one worksheet found in a workbook.
type SheetInfo struct {
	Name string `json:"name"`
	// Rows is the worksheet's declared extent, which for a workbook whose
	// dimension record is missing or stale is an estimate.
	Rows int `json:"rows"`
	// Cells is how many non-empty cells were seen in the sampled head. It is
	// what decides which sheet holds the catalogue.
	Cells int `json:"cells"`
	// Width is the widest sampled row.
	Width  int  `json:"width"`
	Hidden bool `json:"hidden"`
	// Chosen marks the sheet that was read.
	Chosen bool `json:"chosen"`
}

// Source records how a file was decoded, so the review screen can tell the
// vendor which tab and which separator were used before they trust the numbers.
type Source struct {
	Format    Format      `json:"format"`
	Sheet     string      `json:"sheet,omitempty"`
	Sheets    []SheetInfo `json:"sheets,omitempty"`
	Delimiter string      `json:"delimiter,omitempty"`
	Encoding  string      `json:"encoding,omitempty"`
	SizeBytes int         `json:"size_bytes"`
	// TotalRows is the chosen sheet's row count. Estimated says whether it came
	// from the workbook's own dimension record rather than from counting.
	TotalRows int  `json:"total_rows"`
	Estimated bool `json:"estimated"`
}

// Preview is the head of a sheet plus what was learned decoding it.
type Preview struct {
	Source
	// Rows are the sampled rows, padded to Width and indexed from zero, where
	// index i is spreadsheet row i+1.
	Rows [][]string `json:"rows"`
	// Width is the widest row in the sample.
	Width int `json:"width"`
	// Truncated is true when the sheet has more rows than were sampled.
	Truncated bool `json:"truncated"`
}

// RowFunc receives one row during a Walk. index is zero-based and matches the
// spreadsheet's own row numbering minus one, blank rows included, so a finding
// raised here still points at a row in the vendor's copy of the file.
//
// Returning ErrStop ends the walk without an error.
type RowFunc func(index int, row []string) error

// ErrStop ends a Walk early from inside a RowFunc.
var ErrStop = errors.New("sheet: walk stopped")

// ErrEmpty is returned for a file with no readable rows at all.
var ErrEmpty = errors.New("sheet: no rows")
