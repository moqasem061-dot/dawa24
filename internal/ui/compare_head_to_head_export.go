package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// The head-to-head report as a workbook.
//
// It used to be a UTF-8 CSV. A CSV is a text file that Excel happens to open,
// and on this report that cost the reader everything a spreadsheet is for: the
// prices arrived as text that cannot be summed or sorted numerically, the
// discount percentages as strings, and a 300-row comparison arrived as one
// unstyled wall of columns with the supplier names squashed into two unreadable
// headers. The Arabic titles also needed a BOM and still opened as mojibake in
// Excel on Windows when the file was saved and reopened elsewhere.
//
// This writes a real .xlsx: money and percentages as numbers with number
// formats, a frozen, filtered header row, right-to-left sheet direction for the
// Arabic report, and a second sheet carrying the summary. The report itself is
// unchanged — every row that survived the filters is exported, not just the
// page on screen.

// headToHeadExportColumns is the width of the row sheet. Every style range,
// freeze pane and autofilter below is expressed against it, so adding a column
// is one number rather than five literals that can drift apart.
const headToHeadExportColumns = 12

// headToHeadSheetStyles holds the style ids a workbook reuses. Creating a style
// per cell is what makes spreadsheet exports slow: excelize deduplicates them,
// but producing ten thousand identical styles to deduplicate them is not free.
type headToHeadSheetStyles struct {
	header   int
	text     int
	integer  int
	money    int
	percent  int
	label    int
	summaryV int
}

// excelNumber renders an exact amount as the number a spreadsheet stores.
//
// Presentation only: nothing is computed from the result and no money value in
// the application passes through float64. Excel has one numeric type, so a
// price cell has to be a float to stay a price; the value is rendered from the
// amount's own decimal string so the cents are exactly what the report printed,
// and the two-decimal number format keeps Excel from showing binary noise.
func excelNumber(a money.Amount) float64 {
	value, err := strconv.ParseFloat(a.String(), 64)
	if err != nil {
		return 0
	}
	return value
}

// wantsHeadToHeadExport reports whether this request is the download rather
// than the page.
//
// csv is still accepted: it is the spelling every saved link and bookmark
// carries from before this report became a workbook, and someone following one
// should get the report, not a 404. What it downloads is an .xlsx.
func wantsHeadToHeadExport(r *http.Request) bool {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("export"))) {
	case "xlsx", "excel", "csv":
		return true
	default:
		return false
	}
}

// exportHeadToHead streams the full filtered comparison as an Excel workbook.
func (h *UIHandler) exportHeadToHead(w http.ResponseWriter, r *http.Request, report *compare.HeadToHeadComparisonResult) {
	lang := langOf(r)
	now := time.Now()
	book, err := buildHeadToHeadWorkbook(lang, report, now)
	if err != nil {
		h.log.ErrorContext(r.Context(), "build head-to-head workbook", "error", err)
		http.Error(w, i18n.T(lang, "market.search.unavailable"), http.StatusInternalServerError)
		return
	}
	defer func() { _ = book.Close() }()

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, headToHeadFileName(report, now)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Streamed rather than buffered: a comparison of two large price lists is
	// tens of thousands of rows, and holding the whole sheet in memory to set a
	// Content-Length nobody reads is a way to fail the export on a busy node.
	if _, err := book.WriteTo(w); err != nil {
		h.logLoadErr(r.Context(), "write head-to-head workbook", err)
	}
}

// headToHeadFileName is ASCII on purpose: an Arabic filename in a
// Content-Disposition header has to be RFC 5987 encoded, and the browsers that
// read this report do not agree on how. The report's identity is already in the
// sheet title; the file name only has to be unique and obvious.
func headToHeadFileName(report *compare.HeadToHeadComparisonResult, now time.Time) string {
	return fmt.Sprintf("head_to_head_%d_vs_%d_%s.xlsx",
		report.SourceFileID, report.TargetFileID, now.Format("20060102_150405"))
}

// buildHeadToHeadWorkbook assembles the workbook; pure so the export can be
// tested without a response recorder.
func buildHeadToHeadWorkbook(
	lang string, report *compare.HeadToHeadComparisonResult, now time.Time,
) (*excelize.File, error) {
	book := excelize.NewFile()
	sheet := renameFirstSheet(book, i18n.T(lang, "compare.h2h.sheet"))
	if err := book.SetSheetView(sheet, 0, &excelize.ViewOptions{RightToLeft: boolPtr(lang == "ar")}); err != nil {
		return nil, fmt.Errorf("head-to-head workbook: sheet direction: %w", err)
	}

	styles, err := newHeadToHeadStyles(book)
	if err != nil {
		return nil, err
	}
	if err := writeHeadToHeadSheet(book, sheet, lang, report, styles); err != nil {
		return nil, err
	}
	if err := writeHeadToHeadSummary(book, lang, report, now, styles); err != nil {
		return nil, err
	}
	return book, nil
}

func newHeadToHeadStyles(book *excelize.File) (headToHeadSheetStyles, error) {
	var styles headToHeadSheetStyles
	percentFormat := `0.00"%"`
	define := []struct {
		dst  *int
		spec *excelize.Style
	}{
		{&styles.header, &excelize.Style{
			Font:      &excelize.Font{Bold: true, Size: 11, Color: "#FFFFFF"},
			Fill:      excelize.Fill{Type: "pattern", Color: []string{"#0F766E"}, Pattern: 1},
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		}},
		{&styles.text, &excelize.Style{
			Font:      &excelize.Font{Size: 11},
			Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center", WrapText: false},
		}},
		{&styles.integer, &excelize.Style{
			Font:      &excelize.Font{Size: 11},
			NumFmt:    3, // #,##0
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		}},
		{&styles.money, &excelize.Style{
			Font:      &excelize.Font{Size: 11},
			NumFmt:    4, // #,##0.00
			Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "center"},
		}},
		{&styles.percent, &excelize.Style{
			Font:         &excelize.Font{Size: 11},
			CustomNumFmt: &percentFormat,
			Alignment:    &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		}},
		{&styles.label, &excelize.Style{
			Font:      &excelize.Font{Bold: true, Size: 11, Color: "#0F172A"},
			Fill:      excelize.Fill{Type: "pattern", Color: []string{"#F1F5F9"}, Pattern: 1},
			Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"},
		}},
		{&styles.summaryV, &excelize.Style{
			Font:      &excelize.Font{Size: 11},
			Alignment: &excelize.Alignment{Horizontal: "right", Vertical: "center"},
		}},
	}
	for _, d := range define {
		id, err := book.NewStyle(d.spec)
		if err != nil {
			return styles, fmt.Errorf("head-to-head workbook: style: %w", err)
		}
		*d.dst = id
	}
	return styles, nil
}

// writeHeadToHeadSheet writes the header and one row per compared product.
func writeHeadToHeadSheet(
	book *excelize.File, sheet, lang string, report *compare.HeadToHeadComparisonResult,
	styles headToHeadSheetStyles,
) error {
	header := make([]any, 0, headToHeadExportColumns)
	for _, title := range headToHeadHeader(lang, report) {
		header = append(header, title)
	}
	if err := book.SetSheetRow(sheet, "A1", &header); err != nil {
		return fmt.Errorf("head-to-head workbook: header: %w", err)
	}
	if err := book.SetRowHeight(sheet, 1, 32); err != nil {
		return fmt.Errorf("head-to-head workbook: header height: %w", err)
	}
	if err := book.SetCellStyle(sheet, "A1", "L1", styles.header); err != nil {
		return fmt.Errorf("head-to-head workbook: header style: %w", err)
	}

	for idx, row := range report.Rows {
		values := []any{
			idx + 1,
			row.ProductName,
			row.SKU,
			excelNumber(row.Price),
			row.YourDiscount,
			excelNumber(row.YourNetPrice),
			row.CompetitorDiscount,
			excelNumber(row.CompetitorNetPrice),
			headToHeadDifference(row),
			excelNumber(row.SavingsDifference),
			headToHeadOutcomeLabel(lang, row.Outcome),
			row.GroupBasis,
		}
		cell, err := excelize.CoordinatesToCellName(1, idx+2)
		if err != nil {
			return fmt.Errorf("head-to-head workbook: row %d: %w", idx+2, err)
		}
		if err := book.SetSheetRow(sheet, cell, &values); err != nil {
			return fmt.Errorf("head-to-head workbook: row %d: %w", idx+2, err)
		}
	}

	lastRow := len(report.Rows) + 1
	if err := styleHeadToHeadRanges(book, sheet, lastRow, styles); err != nil {
		return err
	}
	if err := book.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		return fmt.Errorf("head-to-head workbook: freeze header: %w", err)
	}
	if len(report.Rows) > 0 {
		// The filter is what makes a 5,000-row comparison usable: the reader
		// narrows to the competitor-wins rows in the sheet itself instead of
		// re-running the page with a tab and exporting it again.
		ref := fmt.Sprintf("A1:L%d", lastRow)
		if err := book.AutoFilter(sheet, ref, nil); err != nil {
			return fmt.Errorf("head-to-head workbook: autofilter: %w", err)
		}
	}
	return book.SetColWidth(sheet, "A", "A", 5)
}

// styleHeadToHeadRanges applies the numeric formats column by column, and the
// text format to the last row so Excel shows the names right-aligned.
func styleHeadToHeadRanges(book *excelize.File, sheet string, lastRow int, styles headToHeadSheetStyles) error {
	widths := map[string]float64{
		"B": 46, "C": 16, "D": 14, "E": 16, "F": 16, "G": 16,
		"H": 16, "I": 14, "J": 14, "K": 22, "L": 18,
	}
	for column, width := range widths {
		if err := book.SetColWidth(sheet, column, column, width); err != nil {
			return fmt.Errorf("head-to-head workbook: column %s: %w", column, err)
		}
	}
	if lastRow < 2 {
		return nil
	}
	formats := []struct {
		column string
		style  int
	}{
		{"A", styles.integer}, {"B", styles.text}, {"C", styles.text},
		{"D", styles.money}, {"E", styles.percent}, {"F", styles.money},
		{"G", styles.percent}, {"H", styles.money}, {"I", styles.percent},
		{"J", styles.money}, {"K", styles.text}, {"L", styles.text},
	}
	for _, f := range formats {
		ref := fmt.Sprintf("%s2:%s%d", f.column, f.column, lastRow)
		if err := book.SetCellStyle(sheet, fmt.Sprintf("%s2", f.column), fmt.Sprintf("%s%d", f.column, lastRow), f.style); err != nil {
			return fmt.Errorf("head-to-head workbook: format %s: %w", ref, err)
		}
	}
	return nil
}

// writeHeadToHeadSummary adds the numbers the page shows above the table, so a
// reader who only received the file still sees who won and by how much.
func writeHeadToHeadSummary(
	book *excelize.File, lang string, report *compare.HeadToHeadComparisonResult,
	now time.Time, styles headToHeadSheetStyles,
) error {
	sheet := i18n.T(lang, "compare.h2h.summary")
	if _, err := book.NewSheet(sheet); err != nil {
		return fmt.Errorf("head-to-head workbook: summary sheet: %w", err)
	}
	if err := book.SetSheetView(sheet, 0, &excelize.ViewOptions{RightToLeft: boolPtr(lang == "ar")}); err != nil {
		return fmt.Errorf("head-to-head workbook: summary direction: %w", err)
	}

	lines := []struct {
		label string
		value any
		style int
	}{
		{i18n.T(lang, "compare.h2h.exported_at"), now.Format("2006-01-02 15:04"), styles.summaryV},
		{i18n.T(lang, "compare.h2h.source_supplier"), report.SourceSupplierName, styles.summaryV},
		{i18n.T(lang, "compare.h2h.target_supplier"), report.TargetSupplierName, styles.summaryV},
		{i18n.T(lang, "compare.h2h.total_shared"), report.TotalShared, styles.integer},
		{i18n.T(lang, "feedback.your_better"), report.YourBetterCount, styles.integer},
		{i18n.T(lang, "feedback.equal"), report.EqualCount, styles.integer},
		{i18n.T(lang, "feedback.competitor_better"), report.CompetitorBetter, styles.integer},
		{i18n.T(lang, "compare.h2h.win_rate"), report.WinRatePercent, styles.percent},
		{i18n.T(lang, "compare.h2h.total_savings"), excelNumber(report.TotalSavings), styles.money},
		{i18n.T(lang, "compare.h2h.source_total"), excelNumber(report.SourceTotal), styles.money},
		{i18n.T(lang, "compare.h2h.target_total"), excelNumber(report.TargetTotal), styles.money},
	}
	for idx, line := range lines {
		rowNum := idx + 1
		labelCell, err := excelize.CoordinatesToCellName(1, rowNum)
		if err != nil {
			return fmt.Errorf("head-to-head workbook: summary row %d: %w", rowNum, err)
		}
		valueCell, err := excelize.CoordinatesToCellName(2, rowNum)
		if err != nil {
			return fmt.Errorf("head-to-head workbook: summary row %d: %w", rowNum, err)
		}
		if err := book.SetCellValue(sheet, labelCell, line.label); err != nil {
			return fmt.Errorf("head-to-head workbook: summary label %d: %w", rowNum, err)
		}
		if err := book.SetCellValue(sheet, valueCell, line.value); err != nil {
			return fmt.Errorf("head-to-head workbook: summary value %d: %w", rowNum, err)
		}
		if err := book.SetCellStyle(sheet, labelCell, labelCell, styles.label); err != nil {
			return fmt.Errorf("head-to-head workbook: summary label style %d: %w", rowNum, err)
		}
		if err := book.SetCellStyle(sheet, valueCell, valueCell, line.style); err != nil {
			return fmt.Errorf("head-to-head workbook: summary value style %d: %w", rowNum, err)
		}
	}
	if err := book.SetColWidth(sheet, "A", "A", 26); err != nil {
		return fmt.Errorf("head-to-head workbook: summary width: %w", err)
	}
	return book.SetColWidth(sheet, "B", "B", 34)
}

// headToHeadHeader is the row sheet's first line: the report's own columns, with
// each supplier's name attached to the discount and net price it explains.
func headToHeadHeader(lang string, report *compare.HeadToHeadComparisonResult) []string {
	currency := i18n.T(lang, "common.currency_egp")
	discount := i18n.T(lang, "feedback.csv.discount")
	net := i18n.T(lang, "feedback.csv.net")
	return []string{
		"#",
		i18n.T(lang, "feedback.csv.product"),
		i18n.T(lang, "feedback.csv.sku"),
		fmt.Sprintf("%s (%s)", i18n.T(lang, "feedback.csv.price"), currency),
		fmt.Sprintf("%s: %s", discount, report.SourceSupplierName),
		fmt.Sprintf("%s (%s): %s", net, currency, report.SourceSupplierName),
		fmt.Sprintf("%s: %s", discount, report.TargetSupplierName),
		fmt.Sprintf("%s (%s): %s", net, currency, report.TargetSupplierName),
		i18n.T(lang, "feedback.csv.difference"),
		fmt.Sprintf("%s (%s)", i18n.T(lang, "feedback.csv.savings"), currency),
		i18n.T(lang, "feedback.outcome"),
		i18n.T(lang, "feedback.csv.basis"),
	}
}

// headToHeadDifference is the discount gap in percentage points — the column
// the report calls the discount difference: the source's discount minus the
// target's.
func headToHeadDifference(row *compare.HeadToHeadRow) float64 {
	return row.YourDiscount - row.CompetitorDiscount
}

// headToHeadOutcomeLabel translates the outcome the report stores as a key.
func headToHeadOutcomeLabel(lang string, outcome compare.HeadToHeadOutcome) string {
	switch outcome {
	case compare.OutcomeYourBetter:
		return i18n.T(lang, "feedback.your_better")
	case compare.OutcomeEqual:
		return i18n.T(lang, "feedback.equal")
	case compare.OutcomeCompetitorBetter:
		return i18n.T(lang, "feedback.competitor_better")
	default:
		return string(outcome)
	}
}
