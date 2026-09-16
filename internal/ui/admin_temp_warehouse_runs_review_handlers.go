package ui

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/importrun"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminTempWarehouseRunReviewPage renders the summary review page.
func (h *UIHandler) AdminTempWarehouseRunReviewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	base := resolveWarehouseRunBase(r)
	runIDStr := chi.URLParam(r, "runID")

	run, err := h.ResolveTempWarehouseRun(ctx, runIDStr)
	if err != nil || run == nil {
		h.redirectWithNotice(w, r, base, "error", "تعذر العثور على جلسة الرفع")
		return
	}

	payload, _ := DecodeTempWarehousePayload(run.Payload)
	sysCtx := database.AsSystem(ctx)

	fileViews := make([]*pages.TempWarehouseMappingFileView, 0, len(payload.FileIDs))
	if h.compareSvc != nil {
		for _, fid := range payload.FileIDs {
			f, fErr := h.compareSvc.GetFile(sysCtx, fid)
			if fErr != nil || f == nil {
				continue
			}
			isMapped := f.MappingConfig.NameCol != nil && *f.MappingConfig.NameCol >= 0
			fileViews = append(fileViews, &pages.TempWarehouseMappingFileView{
				ID:           f.ID,
				Filename:     f.OriginalFilename,
				SupplierName: f.SupplierName,
				RowCount:     f.RowCount,
				Status:       string(f.Status),
				IsMapped:     isMapped,
			})
		}
	}

	runView := &pages.TempWarehouseRunView{
		ID:            run.ID,
		PublicID:      run.PublicID,
		Filename:      run.Filename,
		State:         string(run.State),
		Phase:         PhaseReview,
		Percent:       80,
		TotalFiles:    len(payload.FileIDs),
		BaseURL:       base,
		FileIDs:       payload.FileIDs,
		SupplierNames: payload.SupplierNames,
	}

	h.renderPage(ctx, w, "render run review page", pages.AdminTempWarehouseRunReviewPage(runView, fileViews, lang, dir))
}

// AdminTempWarehouseRunCommitSubmit commits all files in the batch session.
func (h *UIHandler) AdminTempWarehouseRunCommitSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	base := resolveWarehouseRunBase(r)
	runIDStr := chi.URLParam(r, "runID")

	run, err := h.ResolveTempWarehouseRun(ctx, runIDStr)
	if err != nil || run == nil {
		h.redirectWithNotice(w, r, base, "error", "تعذر العثور على جلسة الرفع")
		return
	}

	payload, _ := DecodeTempWarehousePayload(run.Payload)
	sysCtx := database.AsSystem(ctx)

	totalProcessed := 0
	if h.compareSvc != nil {
		for _, fid := range payload.FileIDs {
			f, _ := h.compareSvc.GetFile(sysCtx, fid)
			if f != nil {
				_ = h.compareSvc.ProcessCompareFile(sysCtx, fid)
				totalProcessed += f.RowCount
			}
		}
	}

	if h.importRunRepo != nil {
		_ = h.importRunRepo.UpdateProgress(sysCtx, run.ID, PhaseDone, 100, totalProcessed)
		_ = h.importRunRepo.TransitionState(sysCtx, run.ID, importrun.StateCommitted)
	}

	http.Redirect(w, r, fmt.Sprintf("%s/runs/%d/progress", base, run.ID), http.StatusSeeOther)
}

// AdminTempWarehouseRunProgressPage renders live progress or JSON status polling.
func (h *UIHandler) AdminTempWarehouseRunProgressPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	base := resolveWarehouseRunBase(r)
	runIDStr := chi.URLParam(r, "runID")

	run, err := h.ResolveTempWarehouseRun(ctx, runIDStr)
	if err != nil || run == nil {
		if isJSONOrAJAX(r) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "not found"})
			return
		}
		h.redirectWithNotice(w, r, base, "error", "تعذر العثور على جلسة الرفع")
		return
	}

	if r.URL.Query().Get("poll") == "1" || isJSONOrAJAX(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      run.ID,
			"percent": run.Percent,
			"phase":   run.Phase,
			"state":   run.State,
			"done":    run.Phase == PhaseDone || run.Percent >= 100,
		})
		return
	}

	payload, _ := DecodeTempWarehousePayload(run.Payload)
	runView := &pages.TempWarehouseRunView{
		ID:         run.ID,
		PublicID:   run.PublicID,
		Filename:   run.Filename,
		State:      string(run.State),
		Phase:      run.Phase,
		Percent:    run.Percent,
		TotalFiles: len(payload.FileIDs),
		BaseURL:    base,
	}

	h.renderPage(ctx, w, "render run progress page", pages.AdminTempWarehouseRunProgressPage(runView, lang, dir))
}
