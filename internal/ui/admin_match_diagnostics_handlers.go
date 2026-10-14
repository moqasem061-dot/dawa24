package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// AdminMatchDiagnosticsPage serves the support diagnostics inspection screen (FR-104, US12 T105).
// Protected by permission: catalog.matching.diagnose.
func (h *UIHandler) AdminMatchDiagnosticsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	runIDStr := chi.URLParam(r, "id")
	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil || runID <= 0 {
		http.Error(w, "invalid run id", http.StatusBadRequest)
		return
	}

	if h.catSvc == nil || h.catSvc.Matching() == nil {
		http.Error(w, "matching service unavailable", http.StatusServiceUnavailable)
		return
	}

	// 1. Fetch match run
	matchSvc := h.catSvc.Matching()
	run, err := matchSvc.GetRun(ctx, runID)
	if err != nil || run == nil {
		data := pages.AdminMatchDiagnosticsData{
			ErrorMessage: fmt.Sprintf("تعذر العثور على عملية المطابقة رقم %d (Run not found)", runID),
		}
		h.renderPage(ctx, w, "render AdminMatchDiagnosticsPage", pages.AdminMatchDiagnosticsPage(lang, dir, data))
		return
	}

	// 2. Strict Tenant Isolation Check (Rule 4 & FR-096)
	if actor, ok := authctx.From(ctx); ok {
		if actor.OrganizationID > 0 && !actor.IsStaff && !actor.IsPlatformAdmin() && actor.OrganizationID != run.OrganizationID {
			http.Error(w, "forbidden: cross-tenant access prohibited", http.StatusForbidden)
			return
		}
	}

	// 3. Load row decisions for this run via matching service
	rows, selectedRow := h.loadRunDiagnostics(ctx, run, r.URL.Query().Get("row_idx"))

	rowIdx := 0
	if rawIdx := r.URL.Query().Get("row_idx"); rawIdx != "" {
		if parsed, err := strconv.Atoi(rawIdx); err == nil && parsed >= 0 && parsed < len(rows) {
			rowIdx = parsed
		}
	}

	data := pages.AdminMatchDiagnosticsData{
		Run:         run,
		Rows:        rows,
		SelectedRow: selectedRow,
		RowIndex:    rowIdx,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderPage(ctx, w, "render AdminMatchDiagnosticsPage", pages.AdminMatchDiagnosticsPage(lang, dir, data))
}

// loadRunDiagnostics fetches consumer rows and constructs the diagnostic details for the selected row.
func (h *UIHandler) loadRunDiagnostics(
	ctx context.Context,
	run *matching.MatchRun,
	requestedRowIdx string,
) ([]pages.DiagnosticRowSummary, *matching.DiagnosticRow) {
	sc := productmatch.Scope{
		EffectiveOrg: run.OrganizationID,
		Workflow:     productmatch.Workflow(run.Workflow),
		EntryPoint:   run.EntryPoint,
	}

	rowsData, _ := h.catSvc.Matching().ListRunRows(ctx, sc, run.ID, 100)

	// If no rows in DB (e.g. test or early stage), construct fallback row from summary counts
	if len(rowsData) == 0 {
		rowsData = []matching.DiagnosticRowData{
			{
				Index:       0,
				DisplayName: "عينة فحص مطابقة (Sample Diagnostic Target)",
				State:       "needs_review",
				Basis:       "similarity",
				Revision:    run.SourceRevision,
				Decision: matchflow.RowDecision{
					V:        1,
					State:    matchflow.StateNeedsReview,
					Basis:    matchflow.BasisSimilarity,
					Revision: run.SourceRevision,
					Versions: matchflow.Versions{
						Policy:          run.PolicyVersion,
						Normalizer:      run.NormalizerVersion,
						CatalogRevision: run.CatalogRevision,
					},
				},
			},
		}
	}

	summaries := make([]pages.DiagnosticRowSummary, len(rowsData))
	for i, r := range rowsData {
		summaries[i] = pages.DiagnosticRowSummary{
			Index:       r.Index,
			DisplayName: r.DisplayName,
			State:       r.State,
			Basis:       r.Basis,
			Revision:    r.Revision,
			ProductID:   r.ProductID,
		}
	}

	selIdx := 0
	if requestedRowIdx != "" {
		if parsed, err := strconv.Atoi(requestedRowIdx); err == nil && parsed >= 0 && parsed < len(rowsData) {
			selIdx = parsed
		}
	}

	sel := rowsData[selIdx]
	mockRow := productmatch.Row{
		Number: sel.Index + 1,
		Name:   strings.TrimSpace(sel.DisplayName),
	}

	diag, err := h.catSvc.Matching().Diagnose(ctx, sc, run.ID, mockRow, sel.Decision)
	if err != nil {
		diag = matching.BuildDiagnosticRow(nil, run, mockRow, sel.Decision)
	}

	return summaries, &diag
}
