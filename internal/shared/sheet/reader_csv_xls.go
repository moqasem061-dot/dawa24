package sheet

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/extrame/xls"
)

// reloadSheet re-selects a worksheet in a format decoded whole. Only the legacy
// workbook has more than one, and its grids were discarded after selection, so
// the file is decoded again — a rare path on a small file.
func (b *Book) reloadSheet(name string) error {
	if b.format != FormatXLS {
		return nil
	}
	wb, err := xls.OpenReader(bytes.NewReader(b.content), "utf-8")
	if err != nil || wb == nil {
		return b.fallbackFromBinary(err)
	}
	for i := 0; i < wb.NumSheets(); i++ {
		s := wb.GetSheet(i)
		if s == nil || s.Name != name {
			continue
		}
		b.rows = xlsSheetRows(s)
		b.finishGrid()
		return nil
	}
	return fmt.Errorf("ورقة العمل «%s» غير موجودة في الملف", name)
}

// readCSVRows parses CSV while keeping every row at its true line number.
//
// encoding/csv drops blank lines entirely, so ReadAll returns a slice whose
// indices drift from the file's line numbers the moment a supplier leaves a gap
// between sections — and gaps are exactly what paginated exports are full of.
// Row numbers are promised to match the gutter in the vendor's own spreadsheet,
// so the blanks are restored from the line number the reader itself reports.
func readCSVRows(content []byte, delimiter rune) ([][]string, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	reader.Comma = delimiter
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	// Without this, encoding/csv locks the field count to the first row's and
	// rejects the whole file at the first row that differs. Supplier CSVs are
	// ragged constantly — a trailing note, a short subtotal line — and failing
	// the upload over one of them loses the other nine thousand rows.
	reader.FieldsPerRecord = -1

	var rows [][]string
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if line, _ := reader.FieldPos(0); line > 0 {
			// Pad to where this record began, restoring swallowed blank lines.
			// The bound stops a corrupt line number from allocating wildly.
			if gap := line - 1 - len(rows); gap > 0 && gap <= 10000 {
				for len(rows) < line-1 {
					rows = append(rows, nil)
				}
			}
		}
		rows = append(rows, record)
	}
	return rows, nil
}

// csvHint turns encoding/csv's terse errors into something a vendor can act on
// without knowing what a bare quote is.
func csvHint(err error, filename string) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "quote"):
		return fmt.Sprintf("يوجد خطأ في علامات التنصيص داخل الملف (%s). "+
			"يرجى فتح الملف في Excel وحفظه من جديد بصيغة «CSV UTF-8»", msg)
	case strings.Contains(msg, "wrong number of fields"):
		return fmt.Sprintf("عدد الأعمدة غير متساوٍ بين الصفوف (%s)", msg)
	case filename != "":
		return fmt.Sprintf("%s (الملف: %s)", msg, filename)
	default:
		return msg
	}
}

// decodeText strips a byte-order mark and converts UTF-16 to UTF-8.
//
// Excel's "Unicode Text (*.txt)" export is UTF-16LE with a BOM, and a Windows
// admin who saves a sheet that way gets a file whose every second byte is NUL.
// Read as UTF-8 it yields one unusable column of mojibake.
func decodeText(content []byte) ([]byte, string) {
	switch {
	case bytes.HasPrefix(content, []byte{0xEF, 0xBB, 0xBF}):
		return content[3:], "UTF-8"
	case bytes.HasPrefix(content, []byte{0xFF, 0xFE}):
		return utf16ToUTF8(content[2:], true), "UTF-16LE"
	case bytes.HasPrefix(content, []byte{0xFE, 0xFF}):
		return utf16ToUTF8(content[2:], false), "UTF-16BE"
	}

	// A BOM-less UTF-16LE file still gives itself away: ASCII text puts a NUL
	// in every high byte.
	if len(content) >= 64 && !utf8.Valid(content) {
		nulOdd := 0
		for i := 1; i < 64; i += 2 {
			if content[i] == 0 {
				nulOdd++
			}
		}
		if nulOdd > 24 {
			return utf16ToUTF8(content, true), "UTF-16LE"
		}
	}
	return content, "UTF-8"
}

func utf16ToUTF8(b []byte, little bool) []byte {
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	units := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if little {
			units = append(units, uint16(b[i])|uint16(b[i+1])<<8)
		} else {
			units = append(units, uint16(b[i])<<8|uint16(b[i+1]))
		}
	}
	return []byte(string(utf16.Decode(units)))
}

// binaryControlShare is the proportion of control bytes above which content is
// judged binary rather than dirty text.
//
// A single NUL used to be decisive, and it was the wrong rule: the ERPs that
// export a CSV by dumping fixed-width string records pad them with NUL, so a
// perfectly readable price list was refused with "تعذر التعرف على نوع الملف"
// and the vendor was told their file was corrupt.
//
// A tenth is generous on purpose, because every format that is genuinely binary
// has already been excluded before this runs: Detect matches the ZIP and OLE2
// magic numbers, the XML prolog and the HTML doctype first, so what reaches
// here is either text or something renamed. A PDF or a JPEG is a third control
// bytes; UTF-16 without a byte-order mark is half NUL; scattered padding in a
// price list is under one in fifty.
const binaryControlShare = 0.10

// isBinary reports whether decoded content is not text.
func isBinary(content []byte) bool {
	head := content
	if len(head) > 8192 {
		head = head[:8192]
	}
	if len(head) == 0 {
		return false
	}
	control := 0
	for _, c := range head {
		if c == 0 || (c < 0x20 && c != '\t' && c != '\n' && c != '\r') {
			control++
		}
	}
	if float64(control) > float64(len(head))*binaryControlShare {
		return true
	}
	// Truncating mid-rune at the 8 KB boundary would make valid UTF-8 look
	// invalid, so the tail is only trusted when it is the whole file.
	return len(content) <= 8192 && !utf8.Valid(content)
}

// sniffDelimiter picks the separator that splits the file most consistently.
//
// Counting occurrences on the first line alone misreads any file whose header
// contains a comma inside a quoted label, or whose first line is a title row.
// Consistency across several lines is a far better signal: the real delimiter
// yields the same field count row after row.
func sniffDelimiter(content []byte) rune {
	lines := sampleLines(content, 20)
	if len(lines) == 0 {
		return ','
	}

	bestDelim, bestScore := ',', -1
	for _, delim := range []rune{',', ';', '\t', '|'} {
		counts := make(map[int]int)
		for _, line := range lines {
			if n := countOutsideQuotes(line, delim); n > 0 {
				counts[n+1]++
			}
		}
		// Score the most common field count: how many lines agree, weighted by
		// how many columns that implies. Agreement matters more than width, so
		// a consistent two-column split beats an erratic nine-column one.
		for width, agree := range counts {
			if agree < 2 && len(lines) > 2 {
				continue // a delimiter that only ever worked once is noise
			}
			if score := agree*10 + width; score > bestScore {
				bestDelim, bestScore = delim, score
			}
		}
	}
	// A single column is legitimate — a bare list of product names — and reads
	// the same whichever separator is nominated.
	return bestDelim
}

func sampleLines(content []byte, limit int) []string {
	var out []string
	for _, raw := range bytes.Split(content, []byte("\n")) {
		line := strings.TrimRight(string(raw), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// countOutsideQuotes counts delimiters that are not inside a quoted field, so a
// product name such as "بانادول, أقراص" does not inflate the comma count.
func countOutsideQuotes(line string, delim rune) int {
	n, inQuote := 0, false
	for _, r := range line {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == delim && !inQuote:
			n++
		}
	}
	return n
}
