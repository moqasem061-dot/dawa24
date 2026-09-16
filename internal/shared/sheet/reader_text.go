package sheet

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/extrame/xls"
	"golang.org/x/net/html"
)

// openXLS decodes a legacy Excel 97-2003 workbook.
//
// The previous importer refused these outright and told the vendor to re-save.
// Roughly a fifth of the files real Egyptian distributors send are BIFF, and
// telling a supplier to convert their file is how an import never happens.
//
// Decoding is two-tier: the modern reader (reader_biff.go) reassembles SST
// Continue records correctly and accepts files the legacy decoder glues into
// unreadable blobs; anything it rejects falls through to the legacy path, so
// files that already imported keep importing byte-for-byte as before.
func (b *Book) openXLS() (err error) {
	for _, decode := range []func() error{b.openXLSNative, b.openXLSModern} {
		if decode() == nil {
			return nil
		}
		b.resetGrid()
	}
	if err := b.openXLSLegacy(); err == nil {
		return nil
	}
	b.resetGrid()
	if b.openHTML() == nil {
		b.format, b.source.Format = FormatHTML, FormatHTML
		return nil
	}
	b.resetGrid()
	if b.openXML2003() == nil {
		b.format, b.source.Format = FormatXML2003, FormatXML2003
		return nil
	}
	b.resetGrid()
	return errors.New("تعذر قراءة ملف Excel (.xls) — قد يكون تالفاً أو غير مدعوم. يرجى فتحه في Excel وحفظه بصيغة «مصنف Excel (.xlsx)» ثم إعادة الرفع")
}

// resetGrid discards what a failed decoder left behind.
func (b *Book) resetGrid() {
	b.source.Sheets = nil
	b.source.Sheet = ""
	b.source.TotalRows = 0
	b.rows = nil
}

// openXLSLegacy decodes a legacy Excel 97-2003 workbook with the original
// extrame/xls decoder, kept as the fallback for files the modern reader
// cannot open.
func (b *Book) openXLSLegacy() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = b.fallbackFromBinary(fmt.Errorf("panic in XLS parser: %v", r))
		}
	}()

	wb, err := xls.OpenReader(bytes.NewReader(b.content), "utf-8")
	if err != nil || wb == nil {
		wb, err = xls.OpenReader(bytes.NewReader(b.content), "windows-1256")
	}
	if err != nil || wb == nil {
		return b.fallbackFromBinary(err)
	}

	best, bestCells := -1, 0
	grids := make([][][]string, 0, wb.NumSheets())
	for i := 0; i < wb.NumSheets(); i++ {
		s := wb.GetSheet(i)
		if s == nil {
			grids = append(grids, nil)
			continue
		}
		grid := xlsSheetRows(s)
		grids = append(grids, grid)

		info := SheetInfo{Name: s.Name, Rows: len(grid)}
		for _, row := range grid {
			if len(row) > info.Width {
				info.Width = len(row)
			}
			for _, cell := range row {
				if CleanCell(cell) != "" {
					info.Cells++
				}
			}
		}
		b.source.Sheets = append(b.source.Sheets, info)
		if info.Cells > bestCells {
			bestCells, best = info.Cells, len(b.source.Sheets)-1
		}
	}

	if best < 0 || bestCells == 0 {
		return b.fallbackFromBinary(nil)
	}
	if corruptBIFF(grids[best]) {
		return errors.New("تعذر قراءة هذا الملف بصيغة Excel 97-2003 (.xls) قراءة كاملة — " +
			"جزء كبير من صفوفه غير مقروء. يرجى فتحه في Excel وحفظه بصيغة " +
			"«مصنف Excel (.xlsx)» ثم إعادة الرفع.")
	}
	b.source.Sheets[best].Chosen = true
	b.source.Sheet = b.source.Sheets[best].Name
	b.rows = grids[best]
	b.finishGrid()
	return nil
}

// corruptBIFFShare is the proportion of a sheet's text that may sit inside
// undecodable cells before the workbook is refused rather than partially
// imported.
//
// Measured by length rather than by cell count, because the failure concentrates
// rather than scatters: in the corpus file it glues 735 rows into nine cells, so
// counting cells reports 0.35% corrupt while counting characters reports 94%.
// One in twenty is far above anything a legitimate decode produces and far
// below what this failure produces.
const corruptBIFFShare = 0.05

// corruptBIFF reports whether the BIFF decoder gave back raw record bytes
// instead of strings.
//
// The library this package uses mis-handles Continue records past a certain
// offset in some real files: from that row on, it returns one cell holding the
// undecoded UTF-16 blob of many rows glued together with record separators.
// Nothing downstream can tell that from a very long product name.
//
// A real distributor file in the corpus loses 735 of its 1,011 rows this way,
// and every stage after it behaved perfectly: the columns resolved, 276 rows
// matched at 69%, and the vendor was told the import succeeded. Two thirds of
// their catalogue was simply not there.
//
// So the decode is checked for its own failure signature — a NUL byte, which a
// decoded cell never contains — and a file that shows it is refused with the
// one instruction that actually fixes it. A partial import reported as a whole
// one is the worst outcome available here; an error the vendor can act on is
// the best.
func corruptBIFF(grid [][]string) bool {
	total, corrupt := 0, 0
	for _, row := range grid {
		for _, cell := range row {
			if cell == "" {
				continue
			}
			n := len(cell)
			total += n
			if strings.ContainsRune(cell, 0) {
				corrupt += n
			}
		}
	}
	if total == 0 {
		return false
	}
	return float64(corrupt) > float64(total)*corruptBIFFShare
}

// xlsColumnProbe is how far right a BIFF row is searched for content when the
// workbook's own row extent cannot be trusted, and the hard ceiling on how wide
// a legacy sheet is read.
const xlsColumnProbe = 64

// xlsSheetRows materialises one BIFF worksheet, keeping row positions intact.
func xlsSheetRows(s *xls.WorkSheet) (grid [][]string) {
	defer func() {
		if r := recover(); r != nil {
			// keep whatever rows were collected before panic
		}
	}()
	width := xlsSheetWidth(s)
	grid = make([][]string, 0, int(s.MaxRow)+1)
	for i := 0; i <= int(s.MaxRow); i++ {
		row := s.Row(i)
		if row == nil {
			grid = append(grid, nil)
			continue
		}
		w := max(row.LastCol(), width)
		if w > xlsColumnProbe {
			w = xlsColumnProbe
		}
		cells := make([]string, w)
		for c := 0; c < w; c++ {
			cells[c] = row.Col(c)
		}
		grid = append(grid, cells)
	}
	return grid
}

// xlsSheetWidth finds how many columns a BIFF worksheet actually uses.
//
// Row.LastCol reads the row's declared extent, and a real distributor export
// was found whose every row declares zero while holding four populated cells —
// the writer never filled the field in. Trusting it produced an empty grid and
// a refusal telling the supplier their file was corrupt, when it was not. So
// the declared extent is treated as a lower bound and the head of the sheet is
// searched rightwards for the last cell that actually holds something.
func xlsSheetWidth(s *xls.WorkSheet) int {
	width, probed := 0, 0
	for i := 0; i <= int(s.MaxRow) && probed < 60; i++ {
		row := s.Row(i)
		if row == nil {
			continue
		}
		probed++
		if lc := row.LastCol(); lc > width {
			width = lc
		}
		for c := xlsColumnProbe - 1; c >= width; c-- {
			if CleanCell(row.Col(c)) != "" {
				width = c + 1
				break
			}
		}
	}
	return width
}

// fallbackFromBinary handles a file whose OLE2 signature lied, which happens
// when an ERP writes an HTML table or a CSV and names it .xls.
func (b *Book) fallbackFromBinary(cause error) error {
	if looksLikeHTML(b.content) {
		b.format, b.source.Format = FormatHTML, FormatHTML
		return b.openHTML()
	}
	if cause != nil {
		return fmt.Errorf("تعذر قراءة ملف Excel 97-2003 (.xls) — قد يكون تالفاً أو محمياً. "+
			"يرجى فتحه في Excel وحفظه بصيغة «مصنف Excel (.xlsx)» ثم إعادة الرفع (%v)", cause)
	}
	return errors.New("ملف Excel 97-2003 (.xls) لا يحتوي على بيانات قابلة للقراءة. " +
		"يرجى فتحه في Excel وحفظه بصيغة «مصنف Excel (.xlsx)» ثم إعادة الرفع")
}

// openHTML reads the table an accounting package exported under a spreadsheet
// name. The largest table wins; a page whose real content is in the second
// table after a header banner is common.
func (b *Book) openHTML() error {
	doc, err := html.Parse(bytes.NewReader(b.content))
	if err != nil {
		return fmt.Errorf("تعذر قراءة جدول HTML: %w", err)
	}

	tables := collectTables(doc)
	best, bestCells := -1, 0
	for i, t := range tables {
		cells := 0
		for _, row := range t {
			for _, cell := range row {
				if CleanCell(cell) != "" {
					cells++
				}
			}
		}
		if cells > bestCells {
			bestCells, best = cells, i
		}
	}
	if best < 0 {
		return errors.New("هذا الملف صفحة HTML ولا يحتوي على أي جدول بيانات قابل للاستيراد")
	}

	b.rows = tables[best]
	b.source.Sheet = "HTML"
	b.source.Sheets = []SheetInfo{{Name: "HTML", Rows: len(b.rows), Cells: bestCells, Chosen: true}}
	b.finishGrid()
	return nil
}

// collectTables walks the document and returns each table as a grid.
func collectTables(root *html.Node) [][][]string {
	var tables [][][]string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "table") {
			if grid := tableRows(n); len(grid) > 0 {
				tables = append(tables, grid)
			}
			// Nested tables are layout, not data; do not descend.
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return tables
}

func tableRows(table *html.Node) [][]string {
	var grid [][]string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "tr") {
			var row []string
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (strings.EqualFold(c.Data, "td") || strings.EqualFold(c.Data, "th")) {
					row = append(row, nodeText(c))
				}
			}
			grid = append(grid, row)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(table)
	return grid
}

func nodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(nodeText(c))
	}
	return b.String()
}

// openDelimited decodes CSV and its tab, semicolon and pipe relatives.
func (b *Book) openDelimited(filename string) error {
	content, encoding := decodeText(b.content)
	b.source.Encoding = encoding

	// Anything still binary here is not CSV. Try XLS / XLSX before failing.
	if isBinary(content) {
		if b.openXLS() == nil {
			return nil
		}
		b.resetGrid()
		if b.openXLSX() == nil {
			return nil
		}
		b.resetGrid()
		return errors.New("تعذر التعرف على نوع الملف. الصيغ المدعومة هي Excel (.xlsx و .xls) و CSV والملفات النصية المفصولة")
	}

	delimiter := sniffDelimiter(content)
	rows, err := readCSVRows(content, delimiter)
	if err != nil {
		return fmt.Errorf("تعذر قراءة ملف CSV: %s", csvHint(err, filename))
	}
	if len(rows) == 0 {
		return errors.New("ملف CSV لا يحتوي على أي صفوف")
	}

	b.rows = rows
	b.source.Delimiter = string(delimiter)
	b.source.Sheet = ""
	b.finishGrid()
	return nil
}

// finishGrid records the extent of a fully decoded grid.
func (b *Book) finishGrid() {
	width := 0
	for _, row := range b.rows {
		if len(row) > width {
			width = len(row)
		}
	}
	pad(b.rows, width)
	b.source.TotalRows = len(b.rows)
	b.source.Estimated = false
}
