package ui

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
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

	// Try reading spreadsheet from storage path or upload candidates
	var fileBytes []byte
	storagePath := resolveStoragePath(f.StorageKey, "temp_warehouses")
	if storagePath != "" {
		fileBytes, _ = os.ReadFile(storagePath)
	}

	if len(fileBytes) > 0 {
		rawRows, err := sheet.ReadRows(fileBytes, f.OriginalFilename)
		if err == nil && len(rawRows) > 1 {
			_ = h.compareSvc.DeleteFileRows(ctx, fileID)

			fileRows := make([]*compare.CompareFileRow, 0, len(rawRows)-1)
			for idx, row := range rawRows[1:] {
				if len(row) == 0 {
					continue
				}
				rawName := ""
				if nameCol >= 0 && nameCol < len(row) {
					rawName = strings.TrimSpace(row[nameCol])
				}
				if rawName == "" {
					continue
				}
				sku := ""
				if codeCol >= 0 && codeCol < len(row) {
					sku = strings.TrimSpace(row[codeCol])
				}
				priceMinor := int64(0)
				if priceCol >= 0 && priceCol < len(row) {
					if p, err := parsePriceFloat(row[priceCol]); err == nil && p > 0 {
						priceMinor = int64(math.Round(p * 100))
					}
				}
				discountPct := 0.0
				if discountCol >= 0 && discountCol < len(row) {
					if d, err := parsePriceFloat(row[discountCol]); err == nil && d >= 0 {
						discountPct = d
						if discountPct > 100 {
							discountPct = 100
						}
					}
				}
				priceMoney := money.FromMinor(priceMinor)
				priceAfterMinor := int64(math.Round(float64(priceMinor) * (1.0 - (discountPct / 100.0))))
				priceAfterMoney := money.FromMinor(priceAfterMinor)

				fileRows = append(fileRows, &compare.CompareFileRow{
					FileID:             f.ID,
					OrganizationID:     f.OrganizationID,
					RowNumber:          idx + 2,
					RawName:            rawName,
					NormalizedName:     strings.ToLower(rawName),
					SKU:                sku,
					Price:              priceMoney,
					Discount:           discountPct,
					PriceAfterDiscount: priceAfterMoney,
				})
			}
			if len(fileRows) > 0 {
				_ = h.compareSvc.InsertFileRows(ctx, fileRows)
			}
			f.RowCount = len(fileRows)
		}
	}

	_ = h.compareSvc.UpdateFile(ctx, f)

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

// detectTempWarehouseCols determines column indices based on header names or custom inputs.
func detectTempWarehouseCols(headers []string, customCode, customName, customPrice, customDiscount string) (codeCol, nameCol, priceCol, discountCol int) {
	codeCol, nameCol, priceCol, discountCol = -1, -1, -1, -1
	numCols := len(headers)

	if customCode != "" {
		if c, err := strconv.Atoi(customCode); err == nil && c >= 0 && c < numCols {
			codeCol = c
		}
	}
	if customName != "" {
		if c, err := strconv.Atoi(customName); err == nil && c >= 0 && c < numCols {
			nameCol = c
		}
	}
	if customPrice != "" {
		if c, err := strconv.Atoi(customPrice); err == nil && c >= 0 && c < numCols {
			priceCol = c
		}
	}
	if customDiscount != "" {
		if c, err := strconv.Atoi(customDiscount); err == nil && c >= 0 && c < numCols {
			discountCol = c
		}
	}

	for i, h := range headers {
		norm := strings.ToLower(strings.TrimSpace(h))
		norm = strings.ReplaceAll(norm, "_", "")
		norm = strings.ReplaceAll(norm, "-", "")
		norm = strings.ReplaceAll(norm, " ", "")

		if codeCol == -1 {
			if strings.Contains(norm, i18n.TDefault("w4_ui.s_2_2")) || strings.Contains(norm, "code") || strings.Contains(norm, "sku") ||
				strings.Contains(norm, i18n.TDefault("w4_ui.s_3_3")) || strings.Contains(norm, "barcode") || strings.Contains(norm, i18n.TDefault("w4_ui.s_29_29")) ||
				strings.Contains(norm, "itemcode") {
				codeCol = i
			}
		}

		if nameCol == -1 {
			if strings.Contains(norm, i18n.TDefault("w4_ui.s_30_30")) || strings.Contains(norm, "name") || strings.Contains(norm, i18n.TDefault("w4_ui.s_31_31")) ||
				strings.Contains(norm, i18n.TDefault("w4_ui.s_32_32")) || strings.Contains(norm, "item") || strings.Contains(norm, "product") {
				nameCol = i
			}
		}

		if priceCol == -1 {
			if (strings.Contains(norm, i18n.TDefault("w4_ui.s_33_33")) || strings.Contains(norm, "price") || strings.Contains(norm, i18n.TDefault("w4_ui.s_34_34")) || strings.Contains(norm, i18n.TDefault("w4_ui.s_35_35"))) && !strings.Contains(norm, i18n.TDefault("w4_ui.s_36_36")) && !strings.Contains(norm, i18n.TDefault("w4_ui.s_37_37")) {
				priceCol = i
			}
		}

		if discountCol == -1 {
			if strings.Contains(norm, i18n.TDefault("w4_ui.s_36_36")) || strings.Contains(norm, "discount") || strings.Contains(norm, "%") || strings.Contains(norm, i18n.TDefault("w4_ui.s_38_38")) {
				discountCol = i
			}
		}
	}

	if codeCol == -1 && numCols > 0 {
		codeCol = 0
	}
	if nameCol == -1 && numCols > 1 {
		nameCol = 1
	}
	if priceCol == -1 && numCols > 2 {
		priceCol = 2
	}
	if discountCol == -1 && numCols > 3 {
		discountCol = 3
	}

	return codeCol, nameCol, priceCol, discountCol
}

// parsePriceFloat parses a numeric string into float64, converting Arabic numerals and commas.
func parsePriceFloat(raw string) (float64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_39_39"), "0")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_40_40"), "1")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_41_41"), "2")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_42_42"), "3")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_43_43"), "4")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_44_44"), "5")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_45_45"), "6")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_46_46"), "7")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_47_47"), "8")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_48_48"), "9")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_49_49"), ".")
	s = strings.ReplaceAll(s, "%", "")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_50_50"), "")
	s = strings.ReplaceAll(s, i18n.TDefault("w4_ui.s_51_51"), "")
	s = strings.ReplaceAll(s, "EGP", "")
	s = strings.ReplaceAll(s, "egp", "")
	s = strings.ReplaceAll(s, "LE", "")
	s = strings.ReplaceAll(s, "le", "")
	s = strings.TrimSpace(s)

	if strings.Contains(s, ",") && strings.Contains(s, ".") {
		s = strings.ReplaceAll(s, ",", "")
	} else if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ",", ".")
	}

	var numStr strings.Builder
	hasDot := false
	foundDigit := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			numStr.WriteRune(r)
			foundDigit = true
		} else if r == '.' && !hasDot {
			numStr.WriteRune(r)
			hasDot = true
		} else if foundDigit && r != ' ' && r != '\t' {
			break
		}
	}
	if !foundDigit {
		return 0, fmt.Errorf("no digit found")
	}
	return strconv.ParseFloat(numStr.String(), 64)
}
