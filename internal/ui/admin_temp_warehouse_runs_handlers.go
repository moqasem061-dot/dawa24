package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/importrun"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

// resolveWarehouseRunBase extracts the current route prefix.
func resolveWarehouseRunBase(r *http.Request) string {
	path := r.URL.Path
	if strings.HasPrefix(path, "/admin/temporary-warehouses") {
		return "/admin/temporary-warehouses"
	}
	if strings.Contains(path, "/admin/my/") {
		return "/admin/my/temparte-warehouses"
	}
	if strings.Contains(path, "/admin/team/") {
		return "/admin/team/temparte-warehouses"
	}
	if strings.Contains(path, "/admin/admins/") {
		return "/admin/admins/temparte-warehouses"
	}
	if strings.Contains(path, "/admin/plan/") {
		return "/admin/plan/temparte-warehouses"
	}
	return "/admin/user/temparte-warehouses"
}

// AdminTempWarehouseRunDispatcher redirects a run root URL to its current active phase.
func (h *UIHandler) AdminTempWarehouseRunDispatcher(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	base := resolveWarehouseRunBase(r)
	runIDStr := chi.URLParam(r, "runID")

	run, err := h.ResolveTempWarehouseRun(ctx, runIDStr)
	if err != nil || run == nil {
		h.redirectWithNotice(w, r, base, "error", "تعذر العثور على جلسة الرفع المحددة")
		return
	}

	switch run.Phase {
	case PhaseReview:
		http.Redirect(w, r, fmt.Sprintf("%s/runs/%d/review", base, run.ID), http.StatusSeeOther)
	case PhaseStaging, PhaseCommitting:
		http.Redirect(w, r, fmt.Sprintf("%s/runs/%d/progress", base, run.ID), http.StatusSeeOther)
	case PhaseDone:
		h.redirectWithNotice(w, r, base, "success", "اكتملت جلسة الرفع بنجاح")
	default:
		http.Redirect(w, r, fmt.Sprintf("%s/runs/%d/mapping", base, run.ID), http.StatusSeeOther)
	}
}

// AdminTempWarehouseRunMappingPage renders the full-page column mapping wizard.
func (h *UIHandler) AdminTempWarehouseRunMappingPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	base := resolveWarehouseRunBase(r)
	runIDStr := chi.URLParam(r, "runID")

	run, err := h.ResolveTempWarehouseRun(ctx, runIDStr)
	if err != nil || run == nil {
		h.redirectWithNotice(w, r, base, "error", "تعذر العثور على جلسة الرفع")
		return
	}

	payload, err := DecodeTempWarehousePayload(run.Payload)
	if err != nil || len(payload.FileIDs) == 0 {
		h.redirectWithNotice(w, r, base, "error", "بيانات الجلسة غير صالحة")
		return
	}

	sysCtx := database.AsSystem(ctx)

	activeFileID := payload.CurrentFileID
	if qID, _ := strconv.ParseInt(r.URL.Query().Get("file_id"), 10, 64); qID > 0 {
		for _, fid := range payload.FileIDs {
			if fid == qID {
				activeFileID = qID
				break
			}
		}
	}
	if activeFileID <= 0 && len(payload.FileIDs) > 0 {
		activeFileID = payload.FileIDs[0]
	}

	var curFile *compare.CompareFile
	var curFileView *pages.TempWarehouseMappingFileView
	fileViews := make([]*pages.TempWarehouseMappingFileView, 0, len(payload.FileIDs))

	if h.compareSvc != nil {
		for idx, fid := range payload.FileIDs {
			f, fErr := h.compareSvc.GetFile(sysCtx, fid)
			if fErr != nil || f == nil {
				continue
			}
			if fid == activeFileID {
				curFile = f
				payload.Step = idx + 1
				payload.CurrentFileID = fid
			}

			isMapped := f.MappingConfig.NameCol != nil && *f.MappingConfig.NameCol >= 0
			codeCol, nameCol, priceCol, discountCol := -1, -1, -1, -1
			if f.MappingConfig.CodeCol != nil {
				codeCol = *f.MappingConfig.CodeCol
			}
			if f.MappingConfig.NameCol != nil {
				nameCol = *f.MappingConfig.NameCol
			}
			if f.MappingConfig.PriceCol != nil {
				priceCol = *f.MappingConfig.PriceCol
			}
			if f.MappingConfig.DiscountCol != nil {
				discountCol = *f.MappingConfig.DiscountCol
			}

			fv := &pages.TempWarehouseMappingFileView{
				ID:           f.ID,
				Filename:     f.OriginalFilename,
				SupplierName: f.SupplierName,
				RowCount:     f.RowCount,
				Status:       string(f.Status),
				IsMapped:     isMapped,
				CodeCol:      codeCol,
				NameCol:      nameCol,
				PriceCol:     priceCol,
				DiscountCol:  discountCol,
			}
			fileViews = append(fileViews, fv)
			if fid == activeFileID {
				curFileView = fv
			}
		}
	}

	if curFile == nil && len(payload.FileIDs) > 0 {
		h.redirectWithNotice(w, r, base, "error", "تعذر العثور على ملف المستودع")
		return
	}

	if curFileView != nil {
		headers, preview := h.loadFileHeadersAndPreview(sysCtx, curFile)
		curFileView.Headers = headers
		curFileView.Preview = preview

		if !curFileView.IsMapped && len(headers) > 0 {
			c, n, p, d := detectTempWarehouseCols(headers, "", "", "", "")
			if curFileView.CodeCol < 0 {
				curFileView.CodeCol = c
			}
			if curFileView.NameCol < 0 {
				curFileView.NameCol = n
			}
			if curFileView.PriceCol < 0 {
				curFileView.PriceCol = p
			}
			if curFileView.DiscountCol < 0 {
				curFileView.DiscountCol = d
			}
		}
	}

	runView := &pages.TempWarehouseRunView{
		ID:            run.ID,
		PublicID:      run.PublicID,
		Filename:      run.Filename,
		State:         string(run.State),
		Phase:         run.Phase,
		Percent:       run.Percent,
		Step:          payload.Step,
		TotalFiles:    len(payload.FileIDs),
		CurrentFileID: activeFileID,
		BaseURL:       base,
		FileIDs:       payload.FileIDs,
		SupplierNames: payload.SupplierNames,
	}

	h.renderPage(ctx, w, "render run mapping page", pages.AdminTempWarehouseRunMappingPage(runView, curFileView, fileViews, lang, dir))
}

// AdminTempWarehouseRunMappingSubmit autosaves or commits column choices for a file in the run.
func (h *UIHandler) AdminTempWarehouseRunMappingSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	base := resolveWarehouseRunBase(r)
	runIDStr := chi.URLParam(r, "runID")

	run, err := h.ResolveTempWarehouseRun(ctx, runIDStr)
	if err != nil || run == nil {
		if isJSONOrAJAX(r) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "الجلسة غير موجودة"})
			return
		}
		h.redirectWithNotice(w, r, base, "error", "تعذر العثور على جلسة الرفع")
		return
	}

	if err := r.ParseForm(); err != nil {
		if isJSONOrAJAX(r) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "بيانات الطلب غير صالحة"})
			return
		}
		h.redirectWithNotice(w, r, fmt.Sprintf("%s/runs/%d/mapping", base, run.ID), "error", "خطأ في قراءة البيانات")
		return
	}

	fileID, _ := strconv.ParseInt(r.FormValue("file_id"), 10, 64)
	colName := warehouseMappingColumn(r.FormValue("col_name"))
	colCode := warehouseMappingColumn(r.FormValue("col_code"))
	colPrice := warehouseMappingColumn(r.FormValue("col_price"))
	colDiscount := warehouseMappingColumn(r.FormValue("col_discount"))
	isAutosave := r.FormValue("is_autosave") == "1" || isJSONOrAJAX(r)
	action := r.FormValue("action")

	cfg := compare.MappingConfig{}
	if colName >= 0 {
		cfg.NameCol = &colName
	}
	if colCode >= 0 {
		cfg.CodeCol = &colCode
	}
	if colPrice >= 0 {
		cfg.PriceCol = &colPrice
	}
	if colDiscount >= 0 {
		cfg.DiscountCol = &colDiscount
	}
	if hasDuplicateTempWarehouseColumns(cfg) || action == "review" && !validTempWarehouseMapping(cfg) {
		http.Error(w, i18n.T(lang, "admin.temp_warehouse.mapping_invalid"), http.StatusUnprocessableEntity)
		return
	}

	sysCtx := database.AsSystem(ctx)

	payload, err := DecodeTempWarehousePayload(run.Payload)
	if err != nil || payload == nil || !slices.Contains(payload.FileIDs, fileID) || h.compareSvc == nil || run.State == importrun.StateCancelled || run.State == importrun.StateCommitted {
		http.Error(w, i18n.T(lang, "admin.temp_wh.invalid_id"), http.StatusBadRequest)
		return
	}
	file, err := h.compareSvc.GetFile(sysCtx, fileID)
	if err != nil || file == nil || file.UserID != run.UserID || !file.IsTempWarehouse {
		http.Error(w, i18n.T(lang, "admin.temp_wh.not_found"), http.StatusNotFound)
		return
	}
	if err := h.compareSvc.SaveFileMapping(sysCtx, fileID, cfg); err != nil {
		h.log.ErrorContext(ctx, "warehouse mapping failed", "file_id", fileID, "error", err)
		http.Error(w, i18n.T(lang, "admin.temp_warehouse.insufficient_rows"), http.StatusUnprocessableEntity)
		return
	}
	if payload.Mappings == nil {
		payload.Mappings = make(map[int64]compare.MappingConfig)
	}
	payload.Mappings[fileID] = cfg
	payloadBytes, _ := json.Marshal(payload)
	if err := h.importRunRepo.UpdatePayload(sysCtx, run.ID, payloadBytes); err != nil {
		http.Error(w, i18n.T(lang, "admin.temp_warehouse.insufficient_rows"), http.StatusInternalServerError)
		return
	}

	if isAutosave {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "file_id": fileID})
		return
	}

	if action == "review" {
		if err := h.importRunRepo.UpdateProgress(sysCtx, run.ID, PhaseReview, 70, 0); err != nil {
			http.Error(w, i18n.T(lang, "admin.temp_warehouse.insufficient_rows"), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("%s/runs/%d/review", base, run.ID), http.StatusSeeOther)
		return
	}

	nextFileID := int64(0)
	for i, fid := range payload.FileIDs {
		if fid == fileID && i+1 < len(payload.FileIDs) {
			nextFileID = payload.FileIDs[i+1]
			break
		}
	}

	if nextFileID > 0 {
		payload.CurrentFileID = nextFileID
		pBytes, _ := json.Marshal(payload)
		if err := h.importRunRepo.UpdatePayload(sysCtx, run.ID, pBytes); err != nil {
			http.Error(w, i18n.T(lang, "admin.temp_warehouse.insufficient_rows"), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("%s/runs/%d/mapping?file_id=%d", base, run.ID, nextFileID), http.StatusSeeOther)
		return
	}

	if err := h.importRunRepo.UpdateProgress(sysCtx, run.ID, PhaseReview, 80, 0); err != nil {
		http.Error(w, i18n.T(lang, "admin.temp_warehouse.insufficient_rows"), http.StatusInternalServerError)
		return
	}
	h.redirectWithNotice(w, r, fmt.Sprintf("%s/runs/%d/review", base, run.ID), "success", i18n.T(lang, "admin.temp_wh.all_files_mapped_msg"))
}

// AdminTempWarehouseRunCancelSubmit cancels the run.
func (h *UIHandler) AdminTempWarehouseRunCancelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	base := resolveWarehouseRunBase(r)
	runIDStr := chi.URLParam(r, "runID")

	run, err := h.ResolveTempWarehouseRun(ctx, runIDStr)
	if err != nil || run == nil {
		h.redirectWithNotice(w, r, base, "error", "تعذر العثور على جلسة الرفع")
		return
	}

	sysCtx := database.AsSystem(ctx)
	if err := h.importRunRepo.TransitionState(sysCtx, run.ID, importrun.StateCancelled); err != nil {
		http.Error(w, i18n.T(langOf(r), "admin.temp_warehouse.insufficient_rows"), http.StatusInternalServerError)
		return
	}
	h.redirectWithNotice(w, r, base, "success", "تم إلغاء جلسة الرفع")
}

func warehouseMappingColumn(raw string) int {
	index, err := strconv.Atoi(raw)
	if err != nil || index < 0 || index > 16383 {
		return -1
	}
	return index
}
