package ui

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

func (h *UIHandler) exportHeadToHead(w http.ResponseWriter, r *http.Request, report *compare.HeadToHeadComparisonResult) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="head_to_head_comparison.csv"`)
	if _, err := w.Write([]byte("\xef\xbb\xbf")); err != nil {
		h.logLoadErr(r.Context(), "write CSV BOM", err)
		return
	}
	writer := csv.NewWriter(w)
	if err := writeComparisonCSVRow(writer, headToHeadCSVHeader(langOf(r), report)); err != nil {
		h.logLoadErr(r.Context(), "write CSV header", err)
		return
	}
	for _, row := range report.Rows {
		record := []string{spreadsheetText(row.ProductName), spreadsheetText(row.SKU), row.Price.String(),
			strconv.FormatFloat(row.YourDiscount, 'f', 2, 64), row.YourNetPrice.String(),
			strconv.FormatFloat(row.CompetitorDiscount, 'f', 2, 64), row.CompetitorNetPrice.String(),
			strconv.FormatFloat(row.YourDiscount-row.CompetitorDiscount, 'f', 2, 64), row.SavingsDifference.String(), spreadsheetText(string(row.Outcome)), spreadsheetText(row.GroupBasis)}
		if err := writer.Write(record); err != nil {
			h.logLoadErr(r.Context(), "write head-to-head CSV", err)
			return
		}
	}
	writer.Flush()
	h.logLoadErr(r.Context(), "flush head-to-head CSV", writer.Error())
}

func headToHeadCSVHeader(lang string, report *compare.HeadToHeadComparisonResult) []string {
	discount := i18n.T(lang, "feedback.csv.discount")
	net := i18n.T(lang, "feedback.csv.net")
	return []string{
		i18n.T(lang, "feedback.csv.product"), i18n.T(lang, "feedback.csv.sku"), i18n.T(lang, "feedback.csv.price"),
		discount + ": " + report.SourceSupplierName, net + ": " + report.SourceSupplierName,
		discount + ": " + report.TargetSupplierName, net + ": " + report.TargetSupplierName,
		i18n.T(lang, "feedback.csv.difference"), i18n.T(lang, "feedback.csv.savings"),
		i18n.T(lang, "feedback.outcome"), i18n.T(lang, "feedback.csv.basis"),
	}
}

func writeComparisonCSVRow(writer *csv.Writer, row []string) error {
	for i := range row {
		row[i] = spreadsheetText(row[i])
	}
	return writer.Write(row)
}

func spreadsheetText(text string) string {
	trimmed := strings.TrimLeft(text, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + text
	}
	return text
}
