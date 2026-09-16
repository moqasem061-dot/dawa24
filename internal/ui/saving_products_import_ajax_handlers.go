package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

func (h *UIHandler) handleSavingProductsPreviewColumnsJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if err := r.ParseMultipartForm(uploadMemoryBudget); err != nil {
		_ = json.NewEncoder(w).Encode(SavingProductsPreviewResponse{Success: false, Error: i18n.T(langOf(r), "customer.saving.import.file_too_large")})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		_ = json.NewEncoder(w).Encode(SavingProductsPreviewResponse{Success: false, Error: i18n.T(langOf(r), "customer.saving.import.select_valid_file")})
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		_ = json.NewEncoder(w).Encode(SavingProductsPreviewResponse{Success: false, Error: i18n.T(langOf(r), "customer.saving.import.read_content_error")})
		return
	}

	if err := filesecurity.ValidateSpreadsheetSecurity(fileBytes, header.Filename); err != nil {
		_ = json.NewEncoder(w).Encode(SavingProductsPreviewResponse{Success: false, Error: filesecurity.SecurityErrorMessage})
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, header.Filename)
	if err != nil || len(rawRows) == 0 {
		msg := i18n.T(langOf(r), "customer.saving.import.read_sheets_error")
		if err != nil && strings.Contains(err.Error(), filesecurity.SecurityErrorMessage) {
			msg = filesecurity.SecurityErrorMessage
		}
		_ = json.NewEncoder(w).Encode(SavingProductsPreviewResponse{Success: false, Error: msg})
		return
	}

	headers := rawRows[0]
	var sampleRows [][]string
	if len(rawRows) > 1 {
		limit := 4
		if len(rawRows)-1 < limit {
			limit = len(rawRows) - 1
		}
		sampleRows = rawRows[1 : 1+limit]
	}

	nameCol, skuCol, qtyCol, priceCol, productIDCol := detectSavingProductColumns(headers, sampleRows, "", "", "", "", "")

	_ = json.NewEncoder(w).Encode(SavingProductsPreviewResponse{
		Success: true,
		Headers: headers,
		Detected: SavingDetectedCols{
			NameCol:      nameCol,
			SKUCol:       skuCol,
			QtyCol:       qtyCol,
			PriceCol:     priceCol,
			ProductIDCol: productIDCol,
		},
		SampleRows: sampleRows,
	})
}

func (h *UIHandler) handleSavingProductsImportStartJSON(w http.ResponseWriter, r *http.Request, audience string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "common.unauthorized")})
		return
	}

	if err := parseImportUpload(w, r); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "customer.saving.import.file_too_large")})
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "customer.saving.import.select_valid_file")})
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "customer.saving.import.read_content_error")})
		return
	}

	if err := filesecurity.ValidateSpreadsheetSecurity(fileBytes, fileHeader.Filename); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": filesecurity.SecurityErrorMessage})
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, fileHeader.Filename)
	if err != nil || len(rawRows) <= 1 {
		msg := i18n.T(langOf(r), "customer.saving.import.file_empty_no_rows")
		if err != nil && strings.Contains(err.Error(), filesecurity.SecurityErrorMessage) {
			msg = filesecurity.SecurityErrorMessage
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": msg})
		return
	}

	headers := rawRows[0]
	var sampleRows [][]string
	if len(rawRows) > 1 {
		limit := 4
		if len(rawRows)-1 < limit {
			limit = len(rawRows) - 1
		}
		sampleRows = rawRows[1 : 1+limit]
	}

	nameCol, skuCol, qtyCol, priceCol, productIDCol := detectSavingProductColumns(
		headers,
		sampleRows,
		r.FormValue("col_name"),
		r.FormValue("col_sku"),
		r.FormValue("col_qty"),
		r.FormValue("col_price"),
		r.FormValue("col_product_id"),
	)

	matchChoice := ParseMatchChoice(r)
	useAI := ParseUseAI(r)

	sessionID, totalRows, err := h.startSavingImportRun(
		ctx,
		actor,
		fileHeader.Filename,
		rawRows,
		headers,
		sampleRows,
		nameCol, skuCol, qtyCol, priceCol, productIDCol,
		matchChoice,
		useAI,
		langOf(r),
		audience,
	)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":    true,
		"session_id": sessionID,
		"total_rows": totalRows,
	})
}

func (h *UIHandler) handleSavingProductsImportProgressJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "common.unauthorized")})
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		if h.importRunRepo != nil {
			run, err := h.importRunRepo.GetRunByPublicID(ctx, sessionID, actor.OrganizationID)
			if err == nil && run != nil {
				h.respondWithRunProgress(w, r, run)
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "customer.saving.import.session_not_found")})
		return
	}

	respondWithSavingSessionProgress(w, session)
}

func (h *UIHandler) handleSavingProductsImportCommitJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "common.unauthorized")})
		return
	}

	sessionID := chi.URLParam(r, "id")
	added, updated, err := h.commitSavingImportRun(ctx, sessionID, actor.OrganizationID, actor.UserID, h.catSvc)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": fmt.Sprintf(i18n.T(langOf(r), "customer.saving.import.commit_error"), h.safeMessage(err, langOf(r)))})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"added":   added,
		"updated": updated,
		"message": fmt.Sprintf(i18n.T(langOf(r), "customer.saving.import.commit_success"), added+updated, added, updated),
	})
}

func (h *UIHandler) handleSavingProductsImportCancelJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "common.unauthorized")})
		return
	}

	sessionID := chi.URLParam(r, "id")
	cancelled := globalSavingImportSessionStore.CancelSession(sessionID, actor.OrganizationID)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": cancelled})
}
