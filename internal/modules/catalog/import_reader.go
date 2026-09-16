package catalog

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"

	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// SheetData is a decoded rectangle of cells plus what was learned decoding it.
type SheetData struct {
	Rows []([]string)
	// Sheet is the worksheet the rows came from, empty for CSV.
	Sheet string
	// SheetsSkipped lists other worksheets that held data and were not used.
	// A supplier who put January on one tab and February on another needs to be
	// told only one was read.
	SheetsSkipped []string
	// Format is "xlsx" or "csv".
	Format string
	// Delimiter is the character CSV was split on, empty for Excel.
	Delimiter string
	// Width is the widest row, after ragged rows were padded.
	Width int
}

// Magic prefixes. A ZIP container is an OOXML workbook; the OLE2 compound
// document signature is the legacy BIFF .xls that excelize cannot read.
var (
	magicZIP  = []byte{'P', 'K', 0x03, 0x04}
	magicOLE2 = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
)



// ReadSpreadsheet decodes an uploaded file into rows.
//
// Every failure returns an Arabic message naming the actual problem. The old
// importer surfaced i18n.TDefault("w4_mod.nil_92") — the error was nil because
// the check was `err != nil || len(records) < 1` and an empty file took the
// second branch — which told the admin nothing at all.
func ReadSpreadsheet(content []byte, filename string, opts ...filesecurity.Option) (sd *SheetData, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("تعذر قراءة بيانات الملف (%v)", r)
			sd = nil
		}
	}()

	if len(content) == 0 {
		return nil, errors.New(i18n.T("ar", "err.empty_file"))
	}
	secOpts := append([]filesecurity.Option{}, opts...)
	if err := filesecurity.ValidateSpreadsheetSecurity(content, filename, secOpts...); err != nil {
		return nil, err
	}

	switch {
	case bytes.HasPrefix(content, magicZIP):
		return readExcel(content)

	case bytes.HasPrefix(content, []byte("%PDF")):
		return nil, errors.New(i18n.T("ar", "err.pdf_unsupported"))

	case bytes.HasPrefix(content, magicOLE2), looksLikeHTML(content):
		// Legacy BIFF .xls, and the HTML table a decade-old ERP writes and
		// names .xls. Both used to be refused outright with an instruction to
		// re-save the file, and roughly a fifth of what Egyptian distributors
		// actually send is one or the other — telling a supplier to convert
		// their file is how an import never happens.
		//
		// internal/shared/sheet has decoded both for the vendor import and the
		// smart order all along. This importer had its own reader and its own
		// refusal, which is precisely the drift a shared reader exists to stop:
		// the same file was importable through one screen and rejected by
		// another.
		return readViaSheet(content, filename)
	}

	return readDelimited(content, filename)
}

// readViaSheet decodes through the shared sniffing reader.
//
// It returns the same SheetData the local decoders do, so nothing downstream
// needs to know which reader ran — the row parser, the column mapper and the
// review screen all see one shape.
func readViaSheet(content []byte, filename string) (*SheetData, error) {
	book, err := sheet.Open(content, filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = book.Close() }()

	src := book.Source()
	data := &SheetData{Sheet: src.Sheet, Format: string(src.Format), Delimiter: src.Delimiter}
	for _, info := range src.Sheets {
		if !info.Chosen && info.Cells > 0 {
			data.SheetsSkipped = append(data.SheetsSkipped, info.Name)
		}
	}

	err = book.Walk(func(_ int, row []string) error {
		if len(data.Rows) >= maxSheetRows {
			return errSheetTooLarge
		}
		out := make([]string, len(row))
		for i, cell := range row {
			out[i] = sheet.CleanCell(cell)
		}
		data.Rows = append(data.Rows, out)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(data.Rows) == 0 {
		return nil, errors.New(i18n.T("ar", "err.no_readable_rows"))
	}
	normalizeWidth(data)
	return data, nil
}

func looksLikeHTML(content []byte) bool {
	head := bytes.ToLower(bytes.TrimSpace(content))
	if len(head) > 512 {
		head = head[:512]
	}
	return bytes.HasPrefix(head, []byte("<!doctype html")) ||
		bytes.HasPrefix(head, []byte("<html")) ||
		bytes.HasPrefix(head, []byte("<?xml")) && bytes.Contains(head, []byte("<Workbook"))
}

// Decoding caps. An .xlsx is a ZIP of XML; 32 MB of highly compressible
// workbook decompresses to gigabytes, and GetRows materialises a sheet whole.
// These bound the decode so one malicious — or merely corrupt — upload cannot
// exhaust the process's memory. A real distributor file is tens of thousands
// of rows; both caps are an order of magnitude past that.
const (
	maxSheetRows  = 200_000
	maxSheetCells = 5_000_000
)

// errSheetTooLarge is the refusal for a workbook past the decode caps.
var errSheetTooLarge = errors.New(
	i18n.T("ar", "err.row_limit_exceeded"))

// readExcel picks the worksheet that actually holds the catalogue.
//
// "The sheet with the most rows" is not good enough: exports routinely carry a
// trailing sheet of a thousand blank formatted rows, and a summary tab whose
// twenty rows are the real data. Density — cells that contain something — picks
// the right one, and hidden sheets are skipped because a hidden sheet is
// something the supplier deliberately set aside.
//
// Sheets are scored through the streaming iterator rather than GetRows, so a
// bomb workbook dies against the row cap mid-decode instead of after it has
// been fully materialised in memory. Only the winning sheet is read into the
// slice the parser consumes.
func readExcel(content []byte) (*SheetData, error) {
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf(i18n.TDefault("w4_mod.excel_v_93"), err)
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New(i18n.TDefault("w4_mod.excel_282"))
	}

	type score struct {
		name  string
		rows  int
		cells int
	}
	var best score
	var withData []string

	for _, name := range sheets {
		if visible, vErr := f.GetSheetVisible(name); vErr == nil && !visible {
			continue
		}
		rows, cells, sErr := scoreSheet(f, name)
		if sErr != nil {
			return nil, sErr
		}
		if rows == 0 || cells == 0 {
			continue
		}
		withData = append(withData, name)
		if cells > best.cells {
			best = score{name: name, rows: rows, cells: cells}
		}
	}

	if best.cells == 0 {
		return nil, errors.New(i18n.TDefault("w4_mod.excel_283"))
	}

	rows, err := readRowsCapped(f, best.name)
	if err != nil {
		return nil, err
	}

	data := &SheetData{Rows: rows, Sheet: best.name, Format: "xlsx"}
	for _, name := range withData {
		if name != best.name {
			data.SheetsSkipped = append(data.SheetsSkipped, name)
		}
	}
	normalizeWidth(data)
	return data, nil
}

// scoreSheet counts a sheet's non-empty cells through the streaming iterator,
// enforcing the decode caps as it goes.
func scoreSheet(f *excelize.File, name string) (rows, cells int, err error) {
	iter, err := f.Rows(name)
	if err != nil {
		return 0, 0, nil // unreadable sheet: skip it, like the empty ones
	}
	defer func() { _ = iter.Close() }()

	for iter.Next() {
		rows++
		if rows > maxSheetRows {
			return 0, 0, errSheetTooLarge
		}
		row, err := iter.Columns()
		if err != nil {
			continue
		}
		for _, cell := range row {
			if CleanCellString(cell) != "" {
				cells++
			}
		}
		if cells > maxSheetCells {
			return 0, 0, errSheetTooLarge
		}
	}
	return rows, cells, nil
}

// readRowsCapped reads one sheet whole, refusing past the row cap.
func readRowsCapped(f *excelize.File, name string) ([][]string, error) {
	iter, err := f.Rows(name)
	if err != nil {
		return nil, fmt.Errorf(i18n.TDefault("w4_mod.s_excel_v_94"), name, err)
	}
	defer func() { _ = iter.Close() }()

	rows := make([][]string, 0, min(bestSheetCapacity, maxSheetRows))
	for iter.Next() {
		if len(rows) >= maxSheetRows {
			return nil, errSheetTooLarge
		}
		record, err := iter.Columns()
		if err != nil {
			continue
		}
		rows = append(rows, record)
	}
	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf(i18n.TDefault("w4_mod.excel_v_95"), err)
	}
	return rows, nil
}

// bestSheetCapacity is the initial allocation for the winning sheet: big enough
// for the ordinary case, small enough that a hostile file does not get a huge
// buffer up front.
const bestSheetCapacity = 4096

// readDelimited decodes CSV and its tab/semicolon/pipe relatives.
func readDelimited(content []byte, filename string) (*SheetData, error) {
	content = decodeText(content)

	// Anything still binary at this point is not a spreadsheet in any format we
	// recognise. encoding/csv will happily split a compiled binary on commas and
	// hand back rows of control characters, which the row parser then imports as
	// products — an admin who picks the wrong file deserves a refusal, not a
	// catalogue full of mojibake.
	if isBinary(content) {
		return nil, errors.New(i18n.TDefault("w4_mod.excel_xlsx_csv_284"))
	}

	delimiter, err := sniffDelimiter(content)
	if err != nil {
		return nil, err
	}

	rows, err := readCSVRows(content, delimiter)
	if err != nil {
		return nil, fmt.Errorf(i18n.TDefault("w4_mod.csv_s_96"), csvErrorHint(err, filename))
	}
	if len(rows) == 0 {
		return nil, errors.New(i18n.TDefault("w4_mod.csv_285"))
	}

	data := &SheetData{Rows: rows, Format: "csv", Delimiter: string(delimiter)}
	normalizeWidth(data)
	return data, nil
}
