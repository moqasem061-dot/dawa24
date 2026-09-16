package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CompareBenchmarkOffersModal renders the suppliers behind one count on the
// market-benchmark table.
//
// It is an htmx fragment rather than page data because the alternative is
// carrying every competing offer for every row of a few-thousand-row table into
// the HTML, for the handful a reader actually opens.
func (h *UIHandler) CompareBenchmarkOffersModal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	actor, ok := authctx.From(ctx)
	if !ok || h.compareSvc == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	fileID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("file")), 10, 64)
	rowID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("row")), 10, 64)
	bucket := strings.TrimSpace(r.URL.Query().Get("bucket"))
	if fileID <= 0 || rowID <= 0 || !compare.ValidBenchmarkBucket(bucket) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// The file id arrives in a query string, so ownership is checked here and
	// not assumed from the page that produced the link.
	file, err := h.compareSvc.GetFile(ctx, fileID)
	if err != nil || !h.checkFileOwnership(actor, file) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	data, err := h.compareSvc.BenchmarkRowOffers(ctx, fileID, rowID, compare.BenchmarkBucket(bucket), orgPtr)
	if err != nil {
		h.log.WarnContext(ctx, "benchmark offers modal", "error", err, "file_id", fileID, "row_id", rowID)
		http.Error(w, h.safeMessage(err, lang), http.StatusBadRequest)
		return
	}

	h.renderPage(ctx, w, "render benchmark offers modal", pages.BenchmarkOffersModal(data))
}

// CompareBenchmarkOffersClose empties the modal container. Closing is a swap
// like opening is, so the dialog leaves no markup behind and the next open
// starts from nothing.
func (h *UIHandler) CompareBenchmarkOffersClose(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}
