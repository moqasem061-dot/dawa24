package catalog

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"io"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// readCSVRows parses CSV while keeping every row at its true line number.
//
// encoding/csv drops blank lines entirely, so ReadAll returns a slice whose
// indices drift from the file's line numbers the moment a supplier leaves a gap
// between sections — and gaps are exactly what paginated exports are full of.
// The import report promises that its row numbers match the gutter in the
// admin's own spreadsheet, so the blanks are restored here from the line number
// the reader itself reports.
func readCSVRows(content []byte, delimiter rune) ([][]string, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	reader.Comma = delimiter
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	// Without this, encoding/csv locks the field count to whatever the first row
	// had and rejects the whole file at the first row that differs. Supplier
	// CSVs are ragged constantly — a trailing note, a short subtotal line — and
	// the previous importer failed the entire upload with "record on line 3:
	// wrong number of fields" rather than importing the other 9,000 rows.
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

		// FieldPos reports where this record began. Padding to it restores the
		// blank lines the reader swallowed. A record that legitimately spans
		// several lines through a quoted newline leaves blank rows behind it,
		// which parse as empty and keep every later row number honest.
		//
		// The gap cap exists because a corrupt or hostile file can claim any
		// line number. Rather than padding past it — unbounded memory — or
		// skipping silently, which would shift every later row reference in
		// the report onto the wrong spreadsheet row, an over-wide gap is a
		// refusal the admin can act on.
		if line, _ := reader.FieldPos(0); line > 0 {
			gap := line - 1 - len(rows)
			if gap > maxBlankLineGap {
				return nil, fmt.Errorf(
					i18n.TDefault("w4_mod.d_d_286"),
					maxBlankLineGap, line)
			}
			for len(rows) < line-1 {
				rows = append(rows, nil)
			}
		}
		rows = append(rows, record)
	}
	return rows, nil
}

// maxBlankLineGap is the widest run of blank lines the reader will reconstruct.
const maxBlankLineGap = 1000

// csvErrorHint turns encoding/csv's terse errors into something an admin can
// act on without knowing what a "bare quote" is.
func csvErrorHint(err error, filename string) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "quote"):
		return fmt.Sprintf(i18n.TDefault("w4_mod.s_97")+
			i18n.TDefault("w4_mod.excel_csv_utf_8_98"), msg)
	case strings.Contains(msg, "wrong number of fields"):
		return fmt.Sprintf(i18n.TDefault("w4_mod.s_99"), msg)
	default:
		if filename != "" {
			return fmt.Sprintf(i18n.TDefault("w4_mod.s_s_100"), msg, filename)
		}
		return msg
	}
}

// decodeText strips a byte-order mark and converts UTF-16 to UTF-8.
//
// Excel's "Unicode Text (*.txt)" export is UTF-16LE with a BOM, and a Windows
// admin who saves a sheet that way gets a file whose every second byte is NUL.
// Reading it as UTF-8 yields one unusable column of mojibake, so it is worth the
// few lines to convert properly.
func decodeText(content []byte) []byte {
	switch {
	case bytes.HasPrefix(content, []byte{0xEF, 0xBB, 0xBF}):
		return content[3:]

	case bytes.HasPrefix(content, []byte{0xFF, 0xFE}):
		return utf16ToUTF8(content[2:], true)

	case bytes.HasPrefix(content, []byte{0xFE, 0xFF}):
		return utf16ToUTF8(content[2:], false)
	}

	// A BOM-less UTF-16LE file still gives itself away: ASCII text produces a
	// NUL in every high byte.
	if len(content) >= 64 && !utf8.Valid(content) {
		nulOdd := 0
		for i := 1; i < 64; i += 2 {
			if content[i] == 0 {
				nulOdd++
			}
		}
		if nulOdd > 24 {
			return utf16ToUTF8(content, true)
		}
	}
	return content
}

// isBinary reports whether decoded content is not text.
//
// A NUL byte is decisive — no text encoding this importer accepts produces one
// after decoding — and beyond that a run of control characters or invalid UTF-8
// settles it. Tab, newline and carriage return are text and are not counted.
func isBinary(content []byte) bool {
	head := content
	if len(head) > 8192 {
		head = head[:8192]
	}
	if len(head) == 0 {
		return false
	}
	if bytes.IndexByte(head, 0) >= 0 {
		return true
	}

	control := 0
	for _, b := range head {
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
			control++
		}
	}
	if control*100 > len(head) {
		return true
	}
	// Truncating mid-rune at the 8 KB boundary would make valid UTF-8 look
	// invalid, so the tail is only trusted when it is the whole file.
	return len(content) <= 8192 && !utf8.Valid(content)
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

// sniffDelimiter picks the separator that splits the file most consistently.
//
// Counting occurrences on the first line alone — what the previous importer did
// — misreads any file whose header contains a comma inside a quoted label, or
// whose first line is a title row. Consistency across several lines is a far
// better signal: the real delimiter yields the same field count row after row.
func sniffDelimiter(content []byte) (rune, error) {
	lines := sampleLines(content, 20)
	if len(lines) == 0 {
		return 0, errors.New(i18n.TDefault("w4_mod.csv_287"))
	}

	type result struct {
		delim rune
		score int
		width int
	}
	best := result{delim: ',', score: -1}

	for _, delim := range []rune{',', ';', '\t', '|'} {
		counts := make(map[int]int)
		for _, line := range lines {
			if n := countOutsideQuotes(line, delim); n > 0 {
				counts[n+1]++
			}
		}
		// Score the most common field count: how many lines agree, weighted by
		// how many columns that implies. Agreement matters more than width, so a
		// two-column consistent split beats a nine-column erratic one.
		for width, agree := range counts {
			score := agree*10 + width
			if agree < 2 && len(lines) > 2 {
				continue // a delimiter that only ever worked once is noise
			}
			if score > best.score {
				best = result{delim: delim, score: score, width: width}
			}
		}
	}

	if best.score < 0 {
		// A single column is legitimate: a bare list of product names. Treat it
		// as comma-separated and let row parsing decide whether it is usable.
		return ',', nil
	}
	return best.delim, nil
}

// sampleLines returns up to max non-empty lines from the head of the file.
func sampleLines(content []byte, max int) []string {
	var out []string
	for _, raw := range bytes.Split(content, []byte("\n")) {
		line := strings.TrimRight(string(raw), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= max {
			break
		}
	}
	return out
}

// countOutsideQuotes counts delimiters that are not inside a quoted field, so a
// product name such as i18n.TDefault("w4_mod.s_288_288") does not inflate the comma count.
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

// normalizeWidth pads every row to the width of the widest one.
//
// excelize trims trailing empty cells, so a row whose last three columns are
// blank comes back short. Row parsing then has to bounds-check every access,
// and a mapped column beyond the short row's end silently reads as absent.
// Padding once here makes every row addressable by column index.
func normalizeWidth(d *SheetData) {
	width := 0
	for _, row := range d.Rows {
		if len(row) > width {
			width = len(row)
		}
	}
	d.Width = width
	for i, row := range d.Rows {
		if len(row) < width {
			padded := make([]string, width)
			copy(padded, row)
			d.Rows[i] = padded
		}
	}
}

// ParseUploadedSpreadsheet reads Excel and CSV files into a 2D string slice.
//
// Retained as the simple entry point for callers that need only the cells;
// ReadSpreadsheet additionally reports which sheet and delimiter were chosen,
// which the import report shows so an admin can confirm the right tab was read.
func ParseUploadedSpreadsheet(content []byte, filename string) ([][]string, error) {
	data, err := ReadSpreadsheet(content, filename)
	if err != nil {
		return nil, err
	}
	return data.Rows, nil
}
