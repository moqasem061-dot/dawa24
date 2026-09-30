package ui

import (
	"encoding/csv"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// VendorIngestRowsExport downloads the rows a vendor still has to deal with.
//
// Distinct from VendorIngestExport, which dumps their live inventory: this is
// the ledger of one run, filtered to whatever the results screen was showing.
func (h *UIHandler) VendorIngestRowsExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	publicID := chi.URLParam(r, "id")
	if h.ingSvc == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	filter := ingest.RowFilter{
		Outcome:    r.URL.Query().Get("outcome"),
		MatchLevel: r.URL.Query().Get("match"),
		Limit:      500,
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="import-rows.csv"`)
	// The BOM is what makes Excel open an Arabic CSV as UTF-8 instead of as
	// mojibake, which is the difference between a usable export and a support
	// ticket.
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return
	}

	out := csv.NewWriter(w)
	defer out.Flush()
	_ = out.Write([]string{
		i18n.T(lang, "ingest.csv.row_number"),
		i18n.T(lang, "ingest.csv.item_name"),
		i18n.T(lang, "ingest.csv.matched_catalog_item"),
		i18n.T(lang, "ingest.csv.item_code"),
		i18n.T(lang, "ingest.csv.outcome"),
		i18n.T(lang, "ingest.csv.match_score"),
		i18n.T(lang, "ingest.csv.notes"),
	})

	for offset := 0; offset < 20000; offset += filter.Limit {
		filter.Offset = offset
		rows, total, err := h.ingSvc.ImportRows(ctx, publicID, filter)
		if err != nil || len(rows) == 0 {
			return
		}
		for _, row := range rows {
			_ = out.Write([]string{
				strconv.Itoa(row.SourceRow), row.DisplayName, row.MatchedCatalogName(), row.SourceCode,
				pages.OutcomeLabel(row.Outcome, lang), pages.PercentText(row.MatchScore), row.Message,
			})
		}
		out.Flush()
		if offset+len(rows) >= total {
			return
		}
	}
}
