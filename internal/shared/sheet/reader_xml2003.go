package sheet

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Microsoft Office XML Spreadsheet 2003.
//
// It arrives named `.xls`, so a reader that trusts the extension opens it as
// BIFF and fails, and a reader that sniffs for angle brackets opens it as HTML
// and finds nothing: the document's elements are `<Table>`, `<Row>`, `<Cell>`,
// not `<table>`, `<tr>`, `<td>`, so an HTML parser reports a table with no rows
// in it. That is a silent empty import rather than an error, which is the worst
// of the three outcomes.
//
// Older Egyptian ERPs export this format routinely, so it is parsed properly
// here rather than approximated.

// xml2003Namespace is the marker that identifies the format regardless of what
// the file is called or how its prolog is written.
const xml2003Namespace = "urn:schemas-microsoft-com:office:spreadsheet"

// looksLikeXML2003 reports whether the bytes are an XML Spreadsheet.
//
// The namespace is checked first because it is unambiguous. The fallback — an
// XML prolog with a `<workbook>` element — catches documents whose namespace
// declaration sits past the sniffing window.
func looksLikeXML2003(content []byte) bool {
	head := content
	if len(head) > 4096 {
		head = head[:4096]
	}
	lower := bytes.ToLower(head)
	if bytes.Contains(lower, []byte(xml2003Namespace)) {
		return true
	}
	return bytes.Contains(lower, []byte("<?xml")) && bytes.Contains(lower, []byte("<workbook"))
}

// openXML2003 decodes the largest worksheet in an XML Spreadsheet document.
//
// Read whole, like the other non-OOXML formats: the document is a token stream
// with no index, so there is nothing to seek to and the row count is bounded by
// what the exporting ERP could hold in memory to write it.
func (b *Book) openXML2003() error {
	sheets, err := xml2003Sheets(b.content)
	if err != nil {
		return err
	}

	best, bestCells := -1, 0
	infos := make([]SheetInfo, 0, len(sheets))
	for i, s := range sheets {
		cells := 0
		for _, row := range s.rows {
			for _, cell := range row {
				if CleanCell(cell) != "" {
					cells++
				}
			}
		}
		infos = append(infos, SheetInfo{Name: s.name, Rows: len(s.rows), Cells: cells})
		if cells > bestCells {
			bestCells, best = cells, i
		}
	}
	if best < 0 {
		return errors.New("هذا الملف بصيغة XML Spreadsheet ولا يحتوي على أي جدول بيانات")
	}

	infos[best].Chosen = true
	b.rows = sheets[best].rows
	b.source.Sheet = sheets[best].name
	b.source.Sheets = infos
	b.finishGrid()
	return nil
}

// xml2003Sheet is one decoded worksheet.
type xml2003Sheet struct {
	name string
	rows [][]string
}

// xml2003Sheets decodes every worksheet in the document.
//
// Two attributes carry the sparseness this format uses and both must be
// honoured, or a file with an empty cell shifts every value after it one column
// to the left and the whole mapping is wrong:
//
//   - ss:Index on a Cell states its one-based column, skipping the blanks
//     before it;
//   - ss:MergeAcross states how many further columns this cell spans.
func xml2003Sheets(content []byte) ([]xml2003Sheet, error) {
	dec := xml.NewDecoder(bytes.NewReader(content))
	// Exports from ERPs are not always well-formed by the strict rules, and a
	// file that opens in Excel must open here.
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose
	dec.Entity = xml.HTMLEntity

	var (
		out     []xml2003Sheet
		cur     *xml2003Sheet
		row     []string
		cell    strings.Builder
		inData  bool
		colSpan int
	)

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("تعذر قراءة ملف XML Spreadsheet: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch strings.ToLower(t.Name.Local) {
			case "worksheet":
				out = append(out, xml2003Sheet{name: attrValue(t, "Name")})
				cur = &out[len(out)-1]
			case "row":
				row = make([]string, 0, 16)
			case "cell":
				cell.Reset()
				colSpan = 1 + atoiOr(attrValue(t, "MergeAcross"), 0)
				// A stated index is one-based and counts from the start of the
				// row, so any gap before it is blank cells the export omitted.
				if at := atoiOr(attrValue(t, "Index"), 0); at > 0 {
					for len(row) < at-1 {
						row = append(row, "")
					}
				}
			case "data":
				inData = true
			}

		case xml.EndElement:
			switch strings.ToLower(t.Name.Local) {
			case "worksheet":
				cur = nil
			case "row":
				if cur != nil && !blankStrings(row) {
					cur.rows = append(cur.rows, row)
				}
				row = nil
			case "cell":
				v := CleanCell(cell.String())
				row = append(row, v)
				// A merged cell occupies its extra columns; they are blank so
				// the value is not repeated into fields that did not hold it.
				for i := 1; i < colSpan; i++ {
					row = append(row, "")
				}
				colSpan = 1
			case "data":
				inData = false
			}

		case xml.CharData:
			if inData {
				cell.Write(t)
			}
		}
	}

	if len(out) == 0 {
		return nil, errors.New("هذا الملف بصيغة XML Spreadsheet ولا يحتوي على أي ورقة عمل")
	}
	return out, nil
}

// attrValue reads an attribute by local name, ignoring its namespace prefix —
// the same attribute appears as ss:Name, x:Name or Name depending on the
// exporter.
func attrValue(e xml.StartElement, local string) string {
	for _, a := range e.Attr {
		if strings.EqualFold(a.Name.Local, local) {
			return a.Value
		}
	}
	return ""
}

func atoiOr(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return fallback
	}
	return n
}

// blankStrings reports whether every cell in a row is empty.
func blankStrings(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}
