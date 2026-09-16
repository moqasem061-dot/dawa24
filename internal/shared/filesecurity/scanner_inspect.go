package filesecurity

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/csv"
	"io"
	"path/filepath"
	"strings"

	xreader "github.com/shakinm/xlsReader/xls"
	"github.com/xuri/excelize/v2"
)

// ValidateSpreadsheetSecurity inspects any uploaded spreadsheet (.xlsx, .xls, .csv, text)
// and rejects files containing URLs, web addresses, or domains with ErrSecurityBlocked.
func ValidateSpreadsheetSecurity(content []byte, filename string, opts ...Option) (err error) {
	if len(content) == 0 {
		return nil
	}
	var cfg Options
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.AllowURLs {
		return nil
	}

	defer func() {
		if r := recover(); r != nil {
			// Catch any unexpected panic from third-party binary parsers (e.g. malformed BIFF/XLS records)
			// and fall back to raw content inspection for URLs to maintain security without crashing the handler.
			err = inspectRawContentForURLs(content, cfg)
		}
	}()

	// 1. Detect format by magic bytes or extension
	isZIP := bytes.HasPrefix(content, []byte{'P', 'K', 0x03, 0x04})
	isOLE2 := bytes.HasPrefix(content, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	ext := strings.ToLower(filepath.Ext(filename))

	if isZIP || ext == ".xlsx" || ext == ".xlsm" {
		return inspectXLSX(content, cfg)
	}
	if isOLE2 || ext == ".xls" {
		return inspectXLS(content, cfg)
	}
	return inspectDelimited(content, cfg)
}

func inspectXLSX(content []byte, cfg Options) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = inspectRawContentForURLs(content, cfg)
		}
	}()

	// A. Deep inspect ZIP relationships for hidden external links
	zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err == nil {
		for _, f := range zr.File {
			nameLower := strings.ToLower(f.Name)
			if strings.HasSuffix(nameLower, ".rels") {
				rc, rErr := f.Open()
				if rErr == nil {
					buf, _ := io.ReadAll(io.LimitReader(rc, 2<<20))
					_ = rc.Close()
					bufStr := strings.ToLower(string(buf))
					if strings.Contains(bufStr, "targetmode=\"external\"") ||
						strings.Contains(bufStr, "target=\"http://") ||
						strings.Contains(bufStr, "target=\"https://") ||
						strings.Contains(bufStr, "target=\"ftp://") {
						return ErrSecurityBlocked
					}
				}
			}
		}
	}

	// B. Inspect cell values across all sheets via excelize
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil // Parser error will be handled by the caller
	}
	defer func() { _ = f.Close() }()

	b := newBudget()
	for _, sheetName := range f.GetSheetList() {
		rows, err := f.Rows(sheetName)
		if err != nil {
			continue
		}
		for rows.Next() {
			cols, cErr := rows.Columns()
			if cErr != nil {
				break
			}
			for _, cell := range cols {
				if !b.spend() {
					_ = rows.Close()
					return nil
				}
				if IsSuspiciousText(cell, cfg.AllowEmails) {
					_ = rows.Close()
					return ErrSecurityBlocked
				}
			}
		}
		_ = rows.Close()
	}
	return nil
}

func inspectXLS(content []byte, cfg Options) (err error) {
	defer func() {
		if r := recover(); r != nil {
			// xlsReader may panic on malformed or truncated BIFF records (e.g. slice bounds error).
			// We recover here and fallback to raw content inspection so the file can either be blocked
			// if it contains raw URLs, or passed to downstream parsers (which have multi-tier fallback).
			err = inspectRawContentForURLs(content, cfg)
		}
	}()

	wb, err := xreader.OpenReader(bytes.NewReader(content))
	if err != nil {
		return inspectRawContentForURLs(content, cfg)
	}
	b := newBudget()
	numSheets := wb.GetNumberSheets()
	for s := 0; s < numSheets; s++ {
		sh, serr := wb.GetSheet(s)
		if serr != nil || sh == nil {
			continue
		}
		totalRows := sh.GetNumberRows()
		for r := 0; r <= totalRows; r++ {
			row, rerr := sh.GetRow(r)
			if rerr != nil || row == nil {
				continue
			}
			cols := row.GetCols()
			for _, cell := range cols {
				if cell == nil {
					continue
				}
				if !b.spend() {
					return nil
				}
				if IsSuspiciousText(cell.GetString(), cfg.AllowEmails) {
					return ErrSecurityBlocked
				}
			}
		}
	}
	return nil
}

// splitDelimiter reports whether c separates two fields in a delimited file.
func splitDelimiter(c rune) bool {
	return c == ',' || c == '\t' || c == ';' || c == '|'
}

// isFormulaPrefix reports whether c opens a spreadsheet formula.
// Keep in step with formulaWebRegex's character class.
func isFormulaPrefix(c byte) bool {
	return c == '=' || c == '+' || c == '-' || c == '@'
}

// hasFieldFormulaPrefix reports whether any field on this line begins with a
// formula character, so the line-level gate in inspectDelimited cannot skip a
// line whose formula sits after the first delimiter.
//
// A field begins at the start of the line or just after a delimiter, and
// IsSuspiciousText trims each field before testing it, so leading whitespace
// is skipped here too. All four delimiters and the whitespace this steps over
// are single-byte ASCII, so a byte scan is exact on UTF-8 input.
func hasFieldFormulaPrefix(line string) bool {
	atFieldStart := true
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case splitDelimiter(rune(c)):
			atFieldStart = true
		case c == ' ' || c == '\t' || c == '\r':
			// Whitespace does not end a field's leading run.
		case atFieldStart:
			if isFormulaPrefix(c) {
				return true
			}
			atFieldStart = false
		}
	}
	return false
}

// maxDelimitedLine is the longest single line the fallback scanner will read.
//
// bufio.Scanner defaults to 64 KB and reports an error past it; a price list
// exported as one long line would then be skipped silently. One megabyte is
// past anything a spreadsheet export produces and is still bounded.
const maxDelimitedLine = 1 << 20

func inspectDelimited(content []byte, cfg Options) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = inspectRawContentForURLs(content, cfg)
		}
	}()

	b := newBudget()

	// 1. Try the standard CSV reader.
	r := csv.NewReader(bytes.NewReader(content))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.ReuseRecord = true // the record is consumed before the next Read
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			break // Fallback to line scanning below
		}
		for _, cell := range record {
			if !b.spend() {
				return nil
			}
			if IsSuspiciousText(cell, cfg.AllowEmails) {
				return ErrSecurityBlocked
			}
		}
	}

	// 2. Line scanner for text/tsv/fallback.
	//
	// It streams rather than materialising the file twice. The previous
	// version did `strings.Split(string(content), "\n")`, which allocated a
	// second complete copy of the upload as a string AND a slice header for
	// every line in it — on a 200 MB import batch that is 200 MB of garbage
	// produced to re-read what had just been read.
	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 0, 64<<10), maxDelimitedLine)
	for sc.Scan() {
		trimmed := strings.TrimSpace(sc.Text())
		if trimmed == "" {
			continue
		}
		// One test on the line replaces one test per field, and skips the
		// FieldsFunc allocation entirely for the ordinary case.
		//
		// Two conditions, not one. canCarryAnAddress covers the patterns that
		// match anywhere inside a value, and those survive being split on a
		// delimiter. It does NOT cover formulaWebRegex, which is anchored to
		// the start of a FIELD — "tab,=HYPERLINK(x)" holds no dot, colon or
		// at-sign, so the address test alone would wave it through while the
		// per-field test would have refused it.
		if !canCarryAnAddress(trimmed) && !hasFieldFormulaPrefix(trimmed) {
			continue
		}
		for _, token := range strings.FieldsFunc(trimmed, splitDelimiter) {
			if !b.spend() {
				return nil
			}
			if IsSuspiciousText(token, cfg.AllowEmails) {
				return ErrSecurityBlocked
			}
		}
	}
	return nil
}

// inspectRawContentForURLs scans raw file content bytes for suspicious URLs or external targets
// when a structured parser cannot parse or panics on malformed input.
func inspectRawContentForURLs(content []byte, cfg Options) error {
	if cfg.AllowURLs || len(content) == 0 {
		return nil
	}
	lower := bytes.ToLower(content)
	if bytes.Contains(lower, []byte("http://")) ||
		bytes.Contains(lower, []byte("https://")) ||
		bytes.Contains(lower, []byte("ftp://")) ||
		bytes.Contains(lower, []byte("javascript:")) ||
		bytes.Contains(lower, []byte("data:text")) ||
		wwwRegex.Match(content) {
		return ErrSecurityBlocked
	}
	return nil
}

// Scanned is an upload payload that has passed ValidateSpreadsheetSecurity.
//
// It exists because the compare upload used to scan every file twice: the
// handler scanned it before writing it to disk, and the service scanned the
// same bytes again a few lines later inside RegisterAndStage. On a ten-file
// batch of twenty-thousand-row price lists that was the single most expensive
// thing in the request.
//
// Deleting one of the two calls would have fixed the cost and left the trap:
// nothing in the type system said which layer owned the scan, so the next
// caller of RegisterAndStage would have had to know, and a caller that did not
// would have staged an unscanned file.
//
// A Scanned cannot be constructed except by passing the scan, and holding one
// is proof the scan already ran — so the scan cannot be skipped and cannot be
// repeated. The zero value carries no payload and stages nothing.
type Scanned struct {
	content []byte
}

// Scan validates content and, on success, returns proof of it.
func Scan(content []byte, filename string, opts ...Option) (s Scanned, err error) {
	defer func() {
		if r := recover(); r != nil {
			if secErr := inspectRawContentForURLs(content, Options{}); secErr != nil {
				err = secErr
				s = Scanned{}
			} else {
				err = nil
				s = Scanned{content: content}
			}
		}
	}()
	if err := ValidateSpreadsheetSecurity(content, filename, opts...); err != nil {
		return Scanned{}, err
	}
	return Scanned{content: content}, nil
}

// Bytes returns the validated payload.
func (s Scanned) Bytes() []byte { return s.content }
