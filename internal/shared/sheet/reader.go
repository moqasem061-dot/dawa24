package sheet

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
)

// File decoding.
//
// The format is decided by content, never by the extension. Suppliers rename
// files freely — a .xlsx that is really a CSV, a .csv that is really a tab
// dump, a .xls that is really an HTML table saved by a decade-old ERP — and an
// importer that trusts the name fails on all three while blaming the wrong
// thing.
//
// Reading is two-phase on purpose. Peek touches only the head of the file, so
// the analysis screen can answer "what is in here" for a fifty-megabyte
// workbook without holding fifty megabytes of parsed rows. Walk then streams
// every row past a callback, so the import itself is bounded by one row rather
// than by the file.


// Magic prefixes. A ZIP container is an OOXML workbook; the OLE2 compound
// document signature is the legacy BIFF .xls.
var (
	magicZIP  = []byte{'P', 'K', 0x03, 0x04}
	magicOLE2 = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
)

// isRawBIFF checks for raw unencapsulated BIFF streams (BIFF2 through BIFF8 BOF records).
func isRawBIFF(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	return b[0] == 0x09 && (b[1] == 0x00 || b[1] == 0x02 || b[1] == 0x04 || b[1] == 0x08)
}

// Detect reports which container the bytes are, ignoring the filename.
func Detect(content []byte) Format {
	return DetectWithFilename(content, "")
}

// DetectWithFilename reports the file format using content signatures and filename hints.
func DetectWithFilename(content []byte, filename string) Format {
	clean := bytes.TrimSpace(content)
	clean = bytes.TrimPrefix(clean, []byte{0xEF, 0xBB, 0xBF})

	switch {
	case bytes.HasPrefix(content, magicZIP), bytes.HasPrefix(clean, magicZIP):
		return FormatXLSX
	case bytes.HasPrefix(content, magicOLE2), bytes.HasPrefix(clean, magicOLE2):
		return FormatXLS
	case looksLikeXML2003(content), looksLikeXML2003(clean):
		return FormatXML2003
	case looksLikeHTML(content), looksLikeHTML(clean):
		return FormatHTML
	case isRawBIFF(content), isRawBIFF(clean):
		return FormatXLS
	}

	limit := len(content)
	if limit > 1024 {
		limit = 1024
	}
	if bytes.Contains(content[:limit], magicOLE2) {
		return FormatXLS
	}
	if bytes.Contains(content[:limit], magicZIP) {
		return FormatXLSX
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".xlsx" || ext == ".xlsm" {
		return FormatXLSX
	}
	if ext == ".xls" {
		return FormatXLS
	}

	return FormatCSV
}

func looksLikeHTML(content []byte) bool {
	head := bytes.ToLower(bytes.TrimSpace(content))
	if len(head) > 4096 {
		head = head[:4096]
	}
	switch {
	case bytes.HasPrefix(head, []byte("<!doctype html")), bytes.HasPrefix(head, []byte("<html")):
		return true
	case bytes.HasPrefix(head, []byte("<table")):
		return true
	case bytes.Contains(head, []byte("<table")), bytes.Contains(head, []byte("<html")):
		return true
	}
	return false
}

// Book is an opened file, ready to be sampled and then streamed.
//
// Open decodes only as much as the format forces: a workbook is unzipped and
// its sheet index read, but no worksheet body is parsed until Peek or Walk asks
// for one.
type Book struct {
	format  Format
	content []byte
	source  Source

	// xlsx holds the excelize handle for OOXML workbooks; nil otherwise.
	xlsx *xlsxBook
	// rows holds the fully decoded grid for formats that cannot stream — the
	// legacy BIFF workbook and the HTML table, both of which are parsed whole
	// by their libraries anyway.
	rows [][]string
}

// OpenConfig configures spreadsheet opening.
type OpenConfig struct {
	AllowEmails bool
	// AllowURLs is for the one importer whose file is a list of addresses.
	// See filesecurity.Options.AllowURLs.
	AllowURLs bool
}

// OpenOption modifies OpenConfig.
type OpenOption func(*OpenConfig)

// WithAllowURLs permits addresses in a file whose purpose is to carry them —
// the product-image import, and nothing else.
func WithAllowURLs(allow bool) OpenOption {
	return func(c *OpenConfig) {
		c.AllowURLs = allow
	}
}

// WithAllowEmails permits valid email addresses in spreadsheet cells (e.g. for team member imports).
func WithAllowEmails(allow bool) OpenOption {
	return func(c *OpenConfig) {
		c.AllowEmails = allow
	}
}

// Open decodes a file's container and index.
func Open(content []byte, filename string, opts ...OpenOption) (book *Book, err error) {
	if len(content) == 0 {
		return nil, fmt.Errorf("الملف المرفوع فارغ (0 بايت). يرجى التأكد من اكتمال رفع الملف ثم المحاولة مرة أخرى")
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("تعذر قراءة الملف — قد يكون الملف تالفاً أو غير مدعوم (%v)", r)
			book = nil
		}
	}()

	var cfg OpenConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	var secOpts []filesecurity.Option
	if cfg.AllowEmails {
		secOpts = append(secOpts, filesecurity.WithAllowEmails(true))
	}
	if cfg.AllowURLs {
		secOpts = append(secOpts, filesecurity.WithAllowURLs(true))
	}
	if err := filesecurity.ValidateSpreadsheetSecurity(content, filename, secOpts...); err != nil {
		return nil, err
	}

	actualContent := content
	if limit := len(content); limit > 0 {
		if limit > 1024 {
			limit = 1024
		}
		if idx := bytes.Index(content[:limit], magicOLE2); idx > 0 {
			actualContent = content[idx:]
		}
	}

	detectedFormat := DetectWithFilename(actualContent, filename)
	b := &Book{format: detectedFormat, content: actualContent}
	b.source.Format = b.format
	b.source.SizeBytes = len(actualContent)

	switch b.format {
	case FormatXLSX:
		err = b.openXLSX()
	case FormatXLS:
		err = b.openXLS()
	case FormatHTML:
		err = b.openHTML()
	case FormatXML2003:
		err = b.openXML2003()
	default:
		err = b.openDelimited(filename)
	}
	if err != nil {
		if strings.EqualFold(filepath.Ext(filename), ".xls") && b.format != FormatXLS {
			b.resetGrid()
			if xlsErr := b.openXLS(); xlsErr == nil {
				return b, nil
			}
		}
		return nil, err
	}
	return b, nil
}

// Close releases anything the decoder is holding. Safe on a nil Book.
func (b *Book) Close() error {
	if b == nil || b.xlsx == nil {
		return nil
	}
	return b.xlsx.close()
}

// Source reports how the file was decoded.
func (b *Book) Source() Source { return b.source }

// Sheets lists the worksheets that held data.
func (b *Book) Sheets() []SheetInfo { return b.source.Sheets }

// Use forces a worksheet by name, for a workbook whose densest tab is not the
// one the vendor meant. An unknown name is an error rather than a silent
// fallback: reading the wrong tab is exactly the mistake this guards against.
func (b *Book) Use(name string) error {
	if name == "" || name == b.source.Sheet {
		return nil
	}
	for i := range b.source.Sheets {
		if b.source.Sheets[i].Name != name {
			continue
		}
		for j := range b.source.Sheets {
			b.source.Sheets[j].Chosen = j == i
		}
		b.source.Sheet = name
		b.source.TotalRows = b.source.Sheets[i].Rows
		if b.xlsx == nil {
			return b.reloadSheet(name)
		}
		b.xlsx.sheet = name
		return nil
	}
	return fmt.Errorf("ورقة العمل «%s» غير موجودة في الملف", name)
}

// Peek returns the first maxRows rows of the chosen sheet.
func (b *Book) Peek(maxRows int) (p *Preview, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("تعذر معاينة محتوى الملف — قد يكون الملف تالفاً (%v)", r)
			p = nil
		}
	}()

	if maxRows <= 0 {
		maxRows = DefaultPeekRows
	}
	p = &Preview{Source: b.source}
	err = b.Walk(func(index int, row []string) error {
		if index >= maxRows {
			p.Truncated = true
			return ErrStop
		}
		clean := make([]string, len(row))
		for i, cell := range row {
			clean[i] = CleanCell(cell)
		}
		p.Rows = append(p.Rows, clean)
		if len(clean) > p.Width {
			p.Width = len(clean)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(p.Rows) == 0 {
		return nil, ErrEmpty
	}
	// Trailing blank rows in the sample say nothing and only push the real data
	// out of the window the analyser looks at.
	for len(p.Rows) > 0 && isBlank(p.Rows[len(p.Rows)-1]) {
		p.Rows = p.Rows[:len(p.Rows)-1]
	}
	pad(p.Rows, p.Width)
	if !p.Truncated {
		p.TotalRows = len(p.Rows)
		p.Estimated = false
		p.Source.TotalRows = len(p.Rows)
		p.Source.Estimated = false
	}
	return p, nil
}

// DefaultPeekRows is how much of a sheet the analysis stage reads. It is large
// enough to see past a paginated export's first repeated header and to profile
// a column's values honestly, and small enough that a huge workbook is analysed
// in well under a second.
const DefaultPeekRows = 400

// Walk streams every row of the chosen sheet past fn.
func (b *Book) Walk(fn RowFunc) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("تعذر قراءة صفوف الملف — قد يكون الملف تالفاً (%v)", r)
		}
	}()

	if b.xlsx != nil {
		err = b.xlsx.walk(fn)
	} else {
		err = walkRows(b.rows, fn)
	}
	if errors.Is(err, ErrStop) {
		return nil
	}
	return err
}

func walkRows(rows [][]string, fn RowFunc) error {
	for i, row := range rows {
		if err := fn(i, row); err != nil {
			return err
		}
	}
	return nil
}

func isBlank(row []string) bool {
	for _, cell := range row {
		if CleanCell(cell) != "" {
			return false
		}
	}
	return true
}

// pad widens every row to the same length.
//
// Decoders trim trailing empty cells, so a row whose last three columns are
// blank comes back short. Padding once here makes every row addressable by
// column index instead of forcing a bounds check at every read — and a mapped
// column beyond a short row's end would otherwise read as absent rather than
// empty, which are different things.
func pad(rows [][]string, width int) {
	for i, row := range rows {
		if len(row) >= width {
			continue
		}
		grown := make([]string, width)
		copy(grown, row)
		rows[i] = grown
	}
}
