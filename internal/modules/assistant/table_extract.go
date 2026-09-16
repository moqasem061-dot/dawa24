package assistant

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

const (
	// DefaultMaxTabularRows caps how many rows from a spreadsheet are embedded into the turn context.
	// 150 rows gives plenty of room for medicine order lists while keeping token count reasonable.
	DefaultMaxTabularRows = 150
)

// ReadTabularContent extracts spreadsheet data (XLSX, XLS, CSV, TSV) into a clean Markdown table.
// If the content is not a spreadsheet or is empty, it returns ("", false).
func ReadTabularContent(content []byte, filename string, maxRows int) (string, bool) {
	if len(content) == 0 {
		return "", false
	}
	if maxRows <= 0 {
		maxRows = DefaultMaxTabularRows
	}

	ext := strings.ToLower(filepath.Ext(filename))
	isKnownTabularExt := ext == ".xlsx" || ext == ".xls" || ext == ".csv" || ext == ".tsv"

	detected := sheet.DetectWithFilename(content, filename)
	// If neither the extension nor content detection suggests a spreadsheet, don't attempt sheet decoding.
	if !isKnownTabularExt && detected == "" {
		return "", false
	}

	rows, err := sheet.ReadRows(content, filename)
	if err != nil || len(rows) == 0 {
		return "", false
	}

	// Filter out empty rows and determine max columns
	cleanRows := make([][]string, 0, len(rows))
	maxCols := 0
	for _, r := range rows {
		hasData := false
		for _, cell := range r {
			if strings.TrimSpace(cell) != "" {
				hasData = true
				break
			}
		}
		if hasData {
			if len(r) > maxCols {
				maxCols = len(r)
			}
			cleanRows = append(cleanRows, r)
		}
	}

	if len(cleanRows) == 0 {
		return "", false
	}

	var sb strings.Builder
	totalRows := len(cleanRows)

	// Format as Markdown table
	// First non-empty row as header
	headers := cleanRows[0]
	sb.WriteString("| ")
	for i := 0; i < maxCols; i++ {
		h := ""
		if i < len(headers) {
			h = cleanCellForMarkdown(headers[i])
		}
		if h == "" {
			h = fmt.Sprintf("عمود %d", i+1)
		}
		sb.WriteString(h)
		sb.WriteString(" | ")
	}
	sb.WriteString("\n|")
	for i := 0; i < maxCols; i++ {
		sb.WriteString("---|")
	}
	sb.WriteString("\n")

	limit := totalRows
	if maxRows > 0 && limit > maxRows+1 {
		limit = maxRows + 1
	}

	for _, r := range cleanRows[1:limit] {
		sb.WriteString("| ")
		for i := 0; i < maxCols; i++ {
			c := ""
			if i < len(r) {
				c = cleanCellForMarkdown(r[i])
			}
			sb.WriteString(c)
			sb.WriteString(" | ")
		}
		sb.WriteString("\n")
	}

	if totalRows > limit {
		fmt.Fprintf(&sb, "\n…(تم عرض %d صفاً من أصل %d صفاً في الملف)\n", limit-1, totalRows-1)
	}

	return sb.String(), true
}

func cleanCellForMarkdown(val string) string {
	s := strings.TrimSpace(val)
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}
