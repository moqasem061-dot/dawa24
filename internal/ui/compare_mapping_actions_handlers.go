package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// parseCompareQueue parses remaining file IDs in a setup queue, filtering out the current file ID.
func parseCompareQueue(queueParam string, currentID int64) (int64, string) {
	if queueParam == "" {
		return 0, ""
	}
	rawParts := strings.Split(queueParam, ",")
	var cleaned []string
	for _, part := range rawParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pid, err := strconv.ParseInt(part, 10, 64)
		if err != nil || pid <= 0 {
			continue
		}
		if pid == currentID {
			continue
		}
		cleaned = append(cleaned, part)
	}
	if len(cleaned) == 0 {
		return 0, ""
	}
	nextID, _ := strconv.ParseInt(cleaned[0], 10, 64)
	return nextID, strings.Join(cleaned, ",")
}

// CompareFileMappingSubmit persists user-confirmed column mapping for a spreadsheet.
func (h *UIHandler) CompareFileMappingSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		if r.Header.Get("Accept") == "application/json" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		if r.Header.Get("Accept") == "application/json" {
			http.Error(w, `{"error":"invalid file id"}`, http.StatusBadRequest)
			return
		}
		returnPath := compareReturnPath(r)
		h.redirectWithNotice(w, r, returnPath, "error", i18n.T(lang, "compare.file.invalid_id"))
		return
	}

	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || !h.checkFileOwnership(actor, file) {
			if r.Header.Get("Accept") == "application/json" {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			h.redirectWithNotice(w, r, compareReturnPath(r), "error", i18n.T(lang, "compare.file.edit_forbidden"))
			return
		}

		// Update supplier name if modified in setup wizard
		if newSupplierName := strings.TrimSpace(r.FormValue("supplier_name")); newSupplierName != "" && newSupplierName != file.SupplierName {
			_ = h.compareSvc.RenameFile(ctx, id, newSupplierName)
		}

		var config compare.MappingConfig
		if nameStr := r.FormValue("name_col"); nameStr != "" {
			if idx, err := strconv.Atoi(nameStr); err == nil && idx >= 0 {
				config.NameCol = &idx
			}
		}
		if priceStr := r.FormValue("price_col"); priceStr != "" {
			if idx, err := strconv.Atoi(priceStr); err == nil && idx >= 0 {
				config.PriceCol = &idx
			}
		}
		if discStr := r.FormValue("discount_col"); discStr != "" {
			if idx, err := strconv.Atoi(discStr); err == nil && idx >= 0 {
				config.DiscountCol = &idx
			}
		}
		if codeStr := r.FormValue("code_col"); codeStr != "" {
			if idx, err := strconv.Atoi(codeStr); err == nil && idx >= 0 {
				config.CodeCol = &idx
			}
		}

		if err := h.compareSvc.SaveFileMapping(ctx, id, config); err != nil {
			if r.Header.Get("Accept") == "application/json" {
				http.Error(w, `{"error":"`+h.safeMessage(err, lang)+`"}`, http.StatusInternalServerError)
				return
			}
			h.redirectWithNotice(w, r, compareReturnPath(r), "error", h.safeMessage(err, lang))
			return
		}
	}

	queue := strings.TrimSpace(r.FormValue("setup_queue"))
	if queue == "" {
		queue = strings.TrimSpace(r.FormValue("queue"))
	}
	step, _ := strconv.Atoi(r.FormValue("step"))
	total, _ := strconv.Atoi(r.FormValue("total"))

	nextFileID, nextQueue := parseCompareQueue(queue, id)

	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":         true,
			"next_file_id":    nextFileID,
			"remaining_queue": nextQueue,
			"step":            step + 1,
			"total":           total,
		})
		return
	}

	returnPath := compareReturnPath(r)
	if nextFileID > 0 {
		redirectURL := fmt.Sprintf("%s?setup_file=%d&setup_queue=%s&setup_step=%d&setup_total=%d", returnPath, nextFileID, url.QueryEscape(nextQueue), step+1, total)
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return
	}

	h.redirectWithNotice(w, r, returnPath, "success", "تم حفظ وتطبيق ضبط أعمدة كافة ملفات الموردين، وبدأت عملية المطابقة التلقائية مع الكتالوج في الخلفية بنجاح.")
}

// CompareFileSkipSubmit handles skipping an uploaded file in setup mode.
func (h *UIHandler) CompareFileSkipSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		if r.Header.Get("Accept") == "application/json" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		if r.Header.Get("Accept") == "application/json" {
			http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
			return
		}
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.invalid_id"))
		return
	}
	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err == nil && h.checkFileOwnership(actor, file) {
			_ = h.compareSvc.DeleteFile(ctx, id)
		}
	}

	queue := strings.TrimSpace(r.FormValue("setup_queue"))
	if queue == "" {
		queue = strings.TrimSpace(r.FormValue("queue"))
	}
	step, _ := strconv.Atoi(r.FormValue("step"))
	total, _ := strconv.Atoi(r.FormValue("total"))

	nextFileID, nextQueue := parseCompareQueue(queue, id)

	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":         true,
			"skipped_id":      id,
			"next_file_id":    nextFileID,
			"remaining_queue": nextQueue,
			"step":            step + 1,
			"total":           total,
		})
		return
	}

	if nextFileID > 0 {
		redirectURL := fmt.Sprintf("/compare/tool?setup_file=%d&setup_queue=%s&setup_step=%d&setup_total=%d", nextFileID, url.QueryEscape(nextQueue), step+1, total)
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return
	}

	h.redirectWithNotice(w, r, "/compare/tool", "success", i18n.T(lang, "compare.mapping.skipped_success"))
}

// CompareRowManualMatchSubmit allows users to manually link an uploaded row to a master product.
func (h *UIHandler) CompareRowManualMatchSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}

	rowID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || rowID <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.mapping.invalid_row_id"))
		return
	}

	productID, err := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	if err != nil || productID <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.mapping.valid_product_required"))
		return
	}

	rawName := strings.TrimSpace(r.FormValue("raw_name"))

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	if h.compareSvc != nil {
		if err := h.compareSvc.SaveManualCorrection(ctx, orgPtr, rowID, rawName, productID); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, "/compare/tool", "success", i18n.T(lang, "compare.mapping.match_confirmed_success"))
}

// CompareQuickSearch handles GET /compare/search?q=... and /api/v1/compare/search?q=...
func (h *UIHandler) CompareQuickSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 2 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query":         query,
			"total_matches": 0,
			"items":         []any{},
		})
		return
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}
	if (actor.IsStaff || actor.IsPlatformAdmin()) && orgPtr == nil {
		if orgParam := r.URL.Query().Get("org_id"); orgParam != "" {
			if parsedOrg, err := strconv.ParseInt(orgParam, 10, 64); err == nil && parsedOrg > 0 {
				orgPtr = &parsedOrg
			}
		}
	}

	if h.compareSvc == nil {
		http.Error(w, `{"error":"service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	sysCtx := ctx
	effectiveUserID := actor.UserID
	if orgPtr != nil {
		sysCtx = database.WithTenant(database.AsSystem(ctx), *orgPtr)
		if (actor.IsStaff || actor.IsPlatformAdmin()) && h.orgSvc != nil {
			if targetOrg, err := h.orgSvc.GetOrganization(sysCtx, *orgPtr); err == nil && targetOrg != nil && targetOrg.OwnerID > 0 {
				effectiveUserID = targetOrg.OwnerID
			}
		}
	}

	results, err := h.compareSvc.SearchAcrossSuppliersAndCatalog(sysCtx, effectiveUserID, orgPtr, query)
	if err != nil {
		h.log.ErrorContext(sysCtx, "compare quick search error", "error", err, "query", query)
		results = &compare.CompareSearchResults{
			Query: query,
			Items: []*compare.CompareSearchResultItem{},
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(results)
}
