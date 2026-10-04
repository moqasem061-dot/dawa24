package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// AdminTempWarehouseMappingJSON returns headers and preview rows for a temporary warehouse file.
func (h *UIHandler) AdminTempWarehouseMappingJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	idStr := chi.URLParam(r, "id")
	fileID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || fileID <= 0 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(lang, "admin.temp_wh.invalid_id")})
		return
	}

	file, err := h.compareSvc.GetFile(database.AsSystem(ctx), fileID)
	if err != nil || file == nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(lang, "admin.temp_wh.not_found")})
		return
	}

	headers, preview := h.loadFileHeadersAndPreview(database.AsSystem(ctx), file)
	codeCol := -1
	if file.MappingConfig.CodeCol != nil {
		codeCol = *file.MappingConfig.CodeCol
	}
	nameCol := -1
	if file.MappingConfig.NameCol != nil {
		nameCol = *file.MappingConfig.NameCol
	}
	priceCol := -1
	if file.MappingConfig.PriceCol != nil {
		priceCol = *file.MappingConfig.PriceCol
	}
	discountCol := -1
	if file.MappingConfig.DiscountCol != nil {
		discountCol = *file.MappingConfig.DiscountCol
	}

	if (codeCol < 0 || nameCol < 0 || priceCol < 0 || discountCol < 0) && len(headers) > 0 {
		c, n, p, d := detectTempWarehouseCols(headers, "", "", "", "")
		if codeCol < 0 {
			codeCol = c
		}
		if nameCol < 0 {
			nameCol = n
		}
		if priceCol < 0 {
			priceCol = p
		}
		if discountCol < 0 {
			discountCol = d
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":           true,
		"id":                file.ID,
		"supplier_name":     file.SupplierName,
		"original_filename": file.OriginalFilename,
		"row_count":         file.RowCount,
		"headers":           headers,
		"preview":           preview,
		"code_col":          codeCol,
		"name_col":          nameCol,
		"price_col":         priceCol,
		"discount_col":      discountCol,
	})
}

// AdminTempWarehouseMappingSubmit updates column mappings and reparses rows for a warehouse file.
func (h *UIHandler) AdminTempWarehouseMappingSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	lang := langOf(r)
	idStr := chi.URLParam(r, "id")
	fileID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || fileID <= 0 {
		if isJSONOrAJAX(r) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(lang, "admin.temp_wh.invalid_id")})
			return
		}
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", i18n.T(lang, "admin.temp_wh.invalid_id"))
		return
	}

	f, err := h.compareSvc.GetFile(ctx, fileID)
	if err != nil || f == nil {
		if isJSONOrAJAX(r) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(lang, "admin.temp_wh.not_found")})
			return
		}
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", i18n.T(lang, "admin.temp_wh.not_found"))
		return
	}

	if newName := strings.TrimSpace(r.FormValue("supplier_name")); newName != "" && newName != f.SupplierName {
		_ = h.compareSvc.RenameFile(ctx, fileID, newName)
		f.SupplierName = newName
	}

	codeCol, nameCol, priceCol, discountCol := -1, -1, -1, -1
	if s := r.FormValue("col_code"); s != "" {
		if idx, err := strconv.Atoi(s); err == nil && idx >= 0 {
			codeCol = idx
			f.MappingConfig.CodeCol = &codeCol
		}
	}
	if s := r.FormValue("col_name"); s != "" {
		if idx, err := strconv.Atoi(s); err == nil && idx >= 0 {
			nameCol = idx
			f.MappingConfig.NameCol = &nameCol
		}
	}
	if s := r.FormValue("col_price"); s != "" {
		if idx, err := strconv.Atoi(s); err == nil && idx >= 0 {
			priceCol = idx
			f.MappingConfig.PriceCol = &priceCol
		}
	}
	if s := r.FormValue("col_discount"); s != "" {
		if idx, err := strconv.Atoi(s); err == nil && idx >= 0 {
			discountCol = idx
			f.MappingConfig.DiscountCol = &discountCol
		}
	}

	if err := h.compareSvc.SaveFileMapping(ctx, fileID, f.MappingConfig); err != nil {
		h.log.ErrorContext(ctx, "warehouse remapping failed", "file_id", fileID, "error", err)
		http.Error(w, i18n.T(lang, "admin.temp_warehouse.insufficient_rows"), http.StatusUnprocessableEntity)
		return
	}
	f, err = h.compareSvc.GetFile(ctx, fileID)
	if err != nil {
		http.Error(w, i18n.T(lang, "admin.temp_wh.not_found"), http.StatusNotFound)
		return
	}

	queue := strings.TrimSpace(r.FormValue("setup_queue"))
	step, _ := strconv.Atoi(r.FormValue("step"))
	total, _ := strconv.Atoi(r.FormValue("total"))

	var nextFileID int64
	var nextQueue string
	if queue != "" {
		parts := strings.Split(queue, ",")
		if len(parts) > 0 {
			nextFileID, _ = strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
			if len(parts) > 1 {
				nextQueue = strings.Join(parts[1:], ",")
			}
		}
	}

	if isJSONOrAJAX(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":         true,
			"row_count":       f.RowCount,
			"next_file_id":    nextFileID,
			"remaining_queue": nextQueue,
			"step":            step + 1,
			"total":           total,
			"message":         fmt.Sprintf(i18n.T(lang, "admin.temp_wh.mapping_updated_msg"), f.SupplierName, f.RowCount),
		})
		return
	}

	if nextFileID > 0 {
		redirectURL := fmt.Sprintf("/admin/user/temparte-warehouses?setup_file=%d&setup_queue=%s&setup_step=%d&setup_total=%d&notice=success", nextFileID, url.QueryEscape(nextQueue), step+1, total)
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return
	}

	h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "success", fmt.Sprintf(i18n.T(lang, "admin.temp_wh.mapping_applied_msg"), f.SupplierName))
}

func isJSONOrAJAX(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json") || r.Header.Get("X-Requested-With") == "XMLHttpRequest" || r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") != "true"
}
