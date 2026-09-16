package pipeline

import (
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Reading the buyer's file.
//
// The workbook is streamed, never loaded whole: a large file with a dozen
// columns is a lot of strings to hold at once, and rows are handed straight to
// the database in batches anyway.
//
// The header row is found rather than assumed. Pharmacy exports routinely open
// with a title, a date, a branch name and a blank line before the real headers,
// and reading row 1 as the header turns every column into a mystery.

// TargetFields are the columns smart ordering can use.
//
// Deliberately fewer than the vendor importer's: a purchase list carries no
// prices, because the prices are what the system is going to find. The list is
// the presentation order for the mapping screen; what the columns *mean* is
// decided by productmatch.OrderFields, which is the same resolver every other
// importer runs.
var TargetFields = []struct {
	Key      string
	LabelAR  string
	Required bool
}{
	{"product_name", i18n.TDefault("w4_ui.s_53_53"), true},
	{"sku", i18n.TDefault("w4_ui.s_52_52"), false},
	{"barcode", i18n.TDefault("w4_ui.s_84_84"), false},
	{"quantity", i18n.TDefault("w4_mod.s_368_368"), false},
}

// ParsedFile is what step 2 needs to render the mapping screen.
type ParsedFile struct {
	Headers    []string
	HeaderRow  int
	Preview    [][]string
	Detected   map[int]string
	Confidence map[string]float64
	RowCount   int
}

// Inspect opens the file and works out its shape, without staging any rows.
//
// Separated from Stage because the mapping screen needs the headers and a
// preview before the buyer has confirmed anything, and parsing ten thousand rows
// to show five would be wasteful.
func Inspect(content []byte, filename string) (*ParsedFile, error) {
	book, err := sheet.Open(content, filename)
	if err != nil {
		return nil, fmt.Errorf("could not read the file: %w", err)
	}
	defer book.Close()

	preview, err := book.Peek(25)
	if err != nil {
		return nil, fmt.Errorf("could not read the first rows: %w", err)
	}
	if len(preview.Rows) == 0 {
		return nil, fmt.Errorf("the file has no rows")
	}

	analysis, err := productmatch.AnalyzeWith(book, productmatch.AnalyzeOptions{
		Fields: productmatch.OrderFields,
	})
	if err != nil {
		return nil, fmt.Errorf("could not read the columns: %w", err)
	}

	headerRow := analysis.Layout.HeaderRow
	if headerRow < 0 || headerRow >= len(preview.Rows) {
		headerRow = 0
	}
	headers := analysis.Layout.Headers
	if len(headers) == 0 {
		headers = preview.Rows[headerRow]
	}

	body := preview.Rows[headerRow+1:]
	if len(body) > 5 {
		body = body[:5]
	}

	detected, confidence := translateMapping(analysis.Mapping)
	return &ParsedFile{
		Headers:    headers,
		HeaderRow:  headerRow,
		Preview:    body,
		Detected:   detected,
		Confidence: confidence,
	}, nil
}

// orderFieldKeys maps the shared engine's field names onto the keys smart
// ordering has always persisted in smartorder.column_mappings.
//
// The translation exists so replacing the private detector did not require a
// migration: a run started before this change and resumed after it reads the
// same mapping it was given.
var orderFieldKeys = map[productmatch.Field]string{
	productmatch.FieldName:     "product_name",
	productmatch.FieldSKU:      "sku",
	productmatch.FieldBarcode:  "barcode",
	productmatch.FieldQuantity: "quantity",
}

// translateMapping renders a resolved mapping in smart ordering's vocabulary.
func translateMapping(m *productmatch.Mapping) (map[int]string, map[string]float64) {
	detected := make(map[int]string, len(orderFieldKeys))
	confidence := make(map[string]float64, len(orderFieldKeys))
	if m == nil {
		return detected, confidence
	}
	for _, c := range m.Columns {
		key, ok := orderFieldKeys[c.Field]
		if !ok {
			continue
		}
		detected[c.Index] = key
		confidence[key] = c.Score
	}
	return detected, confidence
}

// Stage reads the whole file through a confirmed mapping and returns the lines.
//
// Rows the mapping cannot give a product identity are dropped rather than staged
// as blanks — a spreadsheet's trailing empty rows and its subtotal line are not
// order lines, and staging them would put i18n.TDefault("w4_ui.s_91_91") in front of the matcher.
func Stage(content []byte, filename string, m *smartorder.Mapping,
	runID, orgID int64) ([]*smartorder.Line, error) {

	book, err := sheet.Open(content, filename)
	if err != nil {
		return nil, fmt.Errorf("could not read the file: %w", err)
	}
	defer book.Close()

	nameCol, hasName := m.Column("product_name")
	skuCol, hasSKU := m.Column("sku")
	barcodeCol, hasBarcode := m.Column("barcode")
	qtyCol, hasQty := m.Column("quantity")

	var lines []*smartorder.Line
	var headers []string
	rowNumber := 0

	err = book.Walk(func(index int, row []string) error {
		if index == m.HeaderRow {
			headers = make([]string, len(row))
			copy(headers, row)
			return nil // header row itself
		}
		if index < m.HeaderRow {
			return nil // banner
		}
		if isRepeatedHeader(row, headers) {
			return nil // repeated header row inside the data
		}
		name := cell(row, nameCol, hasName)
		sku := cell(row, skuCol, hasSKU)
		barcode := cell(row, barcodeCol, hasBarcode)

		if name == "" && sku == "" && barcode == "" {
			return nil // nothing identifies a product here
		}
		if smartorder.IsSummaryRow(name) {
			return nil
		}

		rowNumber++
		l := &smartorder.Line{
			RunID:          runID,
			OrganizationID: orgID,
			RowNumber:      rowNumber,
			Raw:            rawOf(row),
			RawName:        name,
			RawSKU:         sku,
			RawBarcode:     barcode,
			MatchMethod:    smartorder.MethodNone,
			Outcome:        smartorder.OutcomeUnmatched,
		}
		if hasQty {
			q := smartorder.ParseQuantity(cell(row, qtyCol, true))
			l.ImportedQty = q.Qty
			l.QtyParseNote = q.Note
		} else {
			defaultVal := 1.0
			l.ImportedQty = &defaultVal
		}
		lines = append(lines, l)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return lines, nil
}

func cell(row []string, col int, ok bool) string {
	if !ok || col < 0 || col >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[col])
}

// rawOf keeps the original cells so the buyer can always see what they uploaded,
// whatever the mapping decided.
func rawOf(row []string) map[string]string {
	out := make(map[string]string, len(row))
	for i, v := range row {
		if v = strings.TrimSpace(v); v != "" {
			out[fmt.Sprintf("%d", i)] = v
		}
	}
	return out
}

// isRepeatedHeader detects rows that are a duplicate copy of the file's header.
//
// Pharmacy exports with multiple sheets or sections repeat the header at each
// section boundary. Left in, these become lines whose raw_name is "Item
// Description" or i18n.TDefault("w4_ui.s_53_53"), which the matcher dutifully tries to resolve.
// They dilute the stats, waste an AI slot, and confuse the buyer's review.
func isRepeatedHeader(row []string, headers []string) bool {
	if len(headers) == 0 || len(row) == 0 {
		return false
	}
	matches := 0
	checked := 0
	for i, h := range headers {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		checked++
		if i < len(row) && strings.EqualFold(strings.TrimSpace(row[i]), h) {
			matches++
		}
	}
	return checked > 0 && matches >= 2
}
