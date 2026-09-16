package ui

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/importprogress"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// SmartOrderProgressPage renders step 3.
//
// Everything shown comes from the database rather than from the request that
// started the run, so a buyer who closed the tab and came back on another device
// sees the true state (FR-027, US8).
func (h *UIHandler) SmartOrderProgressPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}

	// A finished run has no business showing a spinner.
	switch run.Status {
	case smartorder.StatusCompleted, smartorder.StatusStale:
		http.Redirect(w, r, "/customer/smart-order/"+run.PublicID+"/results", http.StatusSeeOther)
		return
	case smartorder.StatusPlaced:
		http.Redirect(w, r, "/customer/smart-order/"+run.PublicID+"/review", http.StatusSeeOther)
		return
	}

	events, _ := h.smartOrderSvc.Events(ctx, run.ID, 0)
	data := pages.SmartOrderProgressData{
		Run:     run,
		Events:  events,
		Failed:  run.Status == smartorder.StatusFailed,
		Percent: smartorder.RunPercent(events),
		Caption: i18n.T(lang, "smartorder.staging_caption"),
		Message: i18n.T(lang, "smartorder.matching_message"),
	}
	if run.Status == smartorder.StatusFailed {
		if run.FailureReason != "" {
			data.Message = run.FailureReason
		} else {
			data.Message = i18n.T(lang, "smartorder.unexpected_error")
		}
	} else if stage := smartorder.CurrentStage(events); stage != "" {
		data.Caption = stage.Label()
		data.AIRunning = stage == smartorder.StageInitialDone ||
			stage == smartorder.StageAIEnhance || stage == smartorder.StageAdjudicate
	}
	// A run that has been claimed but has emitted nothing yet is genuinely at
	// the start, not stalled. Showing a little progress is the honest reading
	// and stops a freshly queued run looking like a dead one.
	if !data.Failed && data.Percent == 0 {
		data.Percent = 2
	}

	h.renderPage(ctx, w, "render smart order progress page", pages.SmartOrderProgressPage(lang, dir, data))
}

// SmartOrderResultsPage renders step 4: matching and supplier results.
func (h *UIHandler) SmartOrderResultsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()

	// The screen opens on what the buyer can actually order.
	//
	// It used to open on "unmatched", which is the opposite of how the tool is
	// used: a pharmacist runs a smart order because it is fast, and the first
	// thing they were shown was the pile of rows that had failed. Everything
	// that failed is still one click away and its counts are on the page — it
	// is simply not the first thing, and it is not mixed in with the lines that
	// are ready.
	match := strings.TrimSpace(q.Get("match"))
	switch {
	case match == "all":
		match = ""
	case match != "":
	case q.Get("outcome") != "":
		match = q.Get("outcome")
	case q.Has("all"):
		match = ""
	default:
		match = "ready_to_order"
	}

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}

	limit := 25
	if l, err := strconv.Atoi(q.Get("limit")); err == nil && (l == 10 || l == 25 || l == 50 || l == 100 || l == -1) {
		limit = l
	}

	sortBy := strings.TrimSpace(q.Get("sort"))
	sortOrder := strings.TrimSpace(q.Get("order"))
	search := strings.TrimSpace(q.Get("q"))

	// Default matched_product to confidence asc (low to high first) if not explicitly sorted
	if (match == "matched_product" || match == "matched") && sortBy == "" {
		sortBy = "confidence"
		sortOrder = "asc"
	}

	// A tab may name a match GROUP ("unmatched", "ready_to_order") or a single
	// OUTCOME ("out_of_stock", "coverage_blocked"). The collapsed panel links
	// by outcome, because "why is this line not ordered" is a question the
	// outcome answers exactly and a group only approximately.
	filter := smartorder.LineFilter{
		MatchGroup: match,
		SortBy:     sortBy,
		SortOrder:  sortOrder,
		Search:     search,
		Limit:      limit,
		Offset:     (page - 1) * limit,
		All:        limit == -1,
	}
	if isLineOutcome(match) {
		filter.MatchGroup = ""
		filter.Outcome = match
	}

	filterCounts, err := h.smartOrderSvc.FilterCounts(ctx, run.ID)
	if err != nil {
		h.log.ErrorContext(ctx, "load smart order filter counts", "run_id", run.ID, "error", err)
		http.Error(w, i18n.T(lang, "errors.data_load_failed"), http.StatusInternalServerError)
		return
	}

	lines, total, err := h.smartOrderSvc.Results(ctx, run, filter)
	if err != nil {
		h.log.ErrorContext(ctx, "load smart order results", "run_id", run.ID, "error", err)
		http.Error(w, i18n.T(lang, "errors.data_load_failed"), http.StatusInternalServerError)
		return
	}

	// What the run will not order, split by why, for the collapsed section.
	// A failure here costs a panel, not the page.
	blocked, err := h.smartOrderSvc.BlockedCounts(ctx, run.ID)
	if err != nil {
		h.log.WarnContext(ctx, "load smart order blocked counts", "run_id", run.ID, "error", err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SmartOrderResultsPage(lang, dir, pages.SmartOrderResultsData{
		Run:       run,
		Counts:    filterCounts,
		Blocked:   blocked,
		Lines:     lines,
		Total:     total,
		Page:      page,
		PerPage:   limit,
		Match:     match,
		SortBy:    sortBy,
		SortOrder: sortOrder,
		Search:    search,
	}).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render smart order results page", "error", err)
	}
}

// isLineOutcome reports whether a tab key names one of the run's own outcomes.
//
// Listed rather than derived so an unknown value from a hand-edited URL selects
// nothing rather than being passed to the query as a filter it cannot satisfy.
func isLineOutcome(key string) bool {
	switch smartorder.Outcome(key) {
	case smartorder.OutcomeNoSupplier, smartorder.OutcomeCoverageBlocked,
		smartorder.OutcomeInstitutionalBlocked, smartorder.OutcomeOutOfStock,
		smartorder.OutcomeBelowMinQty, smartorder.OutcomeZeroQty,
		smartorder.OutcomeQuotaBlocked,
		smartorder.OutcomeRemoved:
		return true
	}
	return false
}

// SmartOrderHistoryPage lists previous runs for this organisation.
func (h *UIHandler) SmartOrderHistoryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || h.smartOrderSvc == nil {
		http.Redirect(w, r, "/customer/smart-order/new", http.StatusSeeOther)
		return
	}
	runs, err := h.smartOrderSvc.History(ctx, actor.OrganizationID, 50, 0)
	if err != nil {
		h.log.ErrorContext(ctx, "smart order history: fetch runs", "error", err, "org_id", actor.OrganizationID)
	}
	if len(runs) == 0 && (r.URL.Path == "/customer/smart-order" || r.URL.Path == "/customer/smart-order/") {
		http.Redirect(w, r, "/customer/smart-order/new", http.StatusSeeOther)
		return
	}
	lang, dir := h.localeAndDir(r)
	h.renderPage(ctx, w, "render smart order history page", pages.SmartOrderHistoryPage(runs, lang, dir))
}

func (h *UIHandler) vendorName(ctx context.Context, orgID int64) string {
	if h.orgSvc == nil {
		return i18n.T("ar", "common.supplier")
	}
	o, err := h.orgSvc.GetOrganization(ctx, orgID)
	if err != nil || o == nil || o.LegalName == "" {
		return i18n.T("ar", "common.supplier")
	}
	return o.LegalName
}

// smartOrderAIState checks whether the AI toggle can honestly be offered.
func (h *UIHandler) smartOrderAIState(ctx context.Context, orgID int64, langOptional ...string) (bool, string) {
	lang := "ar"
	if len(langOptional) > 0 && langOptional[0] != "" {
		lang = langOptional[0]
	}
	if h.aiClient == nil {
		return false, i18n.T(lang, "smartorder.ai_not_enabled")
	}
	if !h.aiClient.Enabled() {
		return false, i18n.T(lang, "smartorder.ai_gateway_down")
	}
	if h.orgSvc == nil || orgID <= 0 {
		return false, i18n.T(lang, "smartorder.ai_org_members_only")
	}

	org, err := h.orgSvc.GetOrganization(ctx, orgID)
	if err != nil || org == nil {
		return false, i18n.T(lang, "smartorder.ai_subscription_check_failed")
	}
	if org.AIVirtualKey == "" {
		return false, i18n.T(lang, "smartorder.ai_key_missing")
	}

	return true, ""
}

// SmartOrderProgressJSON is the poll behind the progress ring.
func (h *UIHandler) SmartOrderProgressJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}

	events, _ := h.smartOrderSvc.Events(ctx, run.ID, 0)
	percent := smartorder.RunPercent(events)
	caption := i18n.T(lang, "smartorder.staging_caption")
	if stage := smartorder.CurrentStage(events); stage != "" {
		caption = stage.Label()
	}

	done := false
	switch run.Status {
	case smartorder.StatusCompleted, smartorder.StatusStale,
		smartorder.StatusPlaced, smartorder.StatusFailed:
		done = true
		percent = importprogress.Complete
	default:
		if percent >= importprogress.Complete {
			percent = importprogress.Complete - 1
		}
		if percent <= 0 {
			percent = 2
		}
	}

	payload := map[string]any{
		"percent": percent,
		"message": caption,
		"status":  run.Status,
		"done":    done,
		"failed":  run.Status == smartorder.StatusFailed,
		"error":   run.FailureReason,
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		h.log.WarnContext(ctx, "smart order progress encode failed", "error", err)
	}
}

// utf8BOM makes Excel open a UTF-8 CSV as Arabic rather than as mojibake.
const utf8BOM = "\xEF\xBB\xBF"

// SmartOrderExportCSV streams the whole run as a spreadsheet.
func (h *UIHandler) SmartOrderExportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}

	lines, _, err := h.smartOrderSvc.Results(ctx, run, smartorder.LineFilter{All: true})
	if err != nil {
		h.log.ErrorContext(ctx, "export smart order results", "run_id", run.ID, "error", err)
		http.Error(w, i18n.T(lang, "smartorder.export_error"), http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("smart-order-%s-%s.csv", run.PublicID, time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = w.Write([]byte(utf8BOM))

	out := csv.NewWriter(w)
	defer out.Flush()

	if lang == "en" {
		_ = out.Write([]string{
			"Row #", "Raw Name", "SKU", "Barcode",
			"Matched Product ID", "Matched Product Name",
			"Match Method", "Confidence", "Quantity", "Status", "Reason",
		})
	} else {
		_ = out.Write([]string{
			i18n.T("ar", "smartorder.export.row_num"), i18n.T("ar", "smartorder.export.raw_name"), i18n.T("ar", "smartorder.export.code"), i18n.T("ar", "smartorder.export.barcode"),
			i18n.T("ar", "smartorder.export.matched_id"), i18n.T("ar", "smartorder.export.matched_name"),
			i18n.T("ar", "smartorder.export.method"), i18n.T("ar", "smartorder.export.confidence"), i18n.T("ar", "smartorder.export.quantity"), i18n.T("ar", "smartorder.export.status"), i18n.T("ar", "smartorder.export.reason"),
		})
	}

	for _, l := range lines {
		matchedID, matchedName := "", ""
		if l.Matched() {
			matchedID = strconv.FormatInt(*l.MatchedProductID, 10)
			matchedName = l.MatchedProductName
		}
		_ = out.Write([]string{
			strconv.Itoa(l.RowNumber),
			l.RawName,
			l.RawSKU,
			l.RawBarcode,
			matchedID,
			matchedName,
			pages.MatchMethodLabel(l.MatchMethod),
			fmt.Sprintf("%.0f%%", l.MatchConfidence*100),
			strconv.FormatFloat(l.EffectiveQty, 'f', -1, 64),
			pages.SmartOrderOutcomeLabel(l.Outcome),
			l.OutcomeReason,
		})
	}
}
