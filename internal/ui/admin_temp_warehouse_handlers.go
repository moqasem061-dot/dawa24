package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// tempWarehouseSuperBase / tempWarehouseSuperPage are the URL roots the Super
// Admin "المستودعات المؤقتة" screen posts to. The "مستودعاتي المرفوعة" screen
// uses the /admin/my/temparte-warehouses mirror (see admin_my_temp_warehouse_handlers.go).
const (
	tempWarehouseSuperBase = "/admin/temporary-warehouses"
	tempWarehouseSuperPage = "/admin/user/temparte-warehouses"
)

// AdminTempWarehousesPage renders the Super Admin temporary warehouses directory:
// moderator uploads plus vendor compare-tool files, with uploader / type filters.
func (h *UIHandler) AdminTempWarehousesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	filter := compare.AdminTempWarehouseFilter{
		Search:    strings.TrimSpace(r.URL.Query().Get("q")),
		Source:    strings.TrimSpace(r.URL.Query().Get("type")),
		SortBy:    strings.TrimSpace(r.URL.Query().Get("sort")),
		SortOrder: strings.TrimSpace(r.URL.Query().Get("order")),
	}
	if s := strings.TrimSpace(r.URL.Query().Get("status")); s != "" {
		st := compare.CompareFileStatus(s)
		filter.Status = &st
	}
	if u := strings.TrimSpace(r.URL.Query().Get("uploader")); u != "" {
		if id, err := strconv.ParseInt(u, 10, 64); err == nil && id > 0 {
			filter.UploaderID = &id
		}
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)

	data := h.buildTempWarehousesData(ctx, filter, false, page, limit)
	data.Base = tempWarehouseSuperBase
	data.PageURL = tempWarehouseSuperPage
	data.NoticeMsg = strings.TrimSpace(r.URL.Query().Get("notice"))
	data.NoticeType = strings.TrimSpace(r.URL.Query().Get("notice_type"))
	if data.NoticeMsg == "" {
		if m := strings.TrimSpace(r.URL.Query().Get("msg")); m != "" {
			data.NoticeMsg = m
		}
	}
	if h.compareSvc != nil {
		if ups, err := h.compareSvc.ListTempWarehouseUploaders(database.AsSystem(ctx)); err == nil {
			for _, u := range ups {
				data.Uploaders = append(data.Uploaders, pages.AdminTempWarehouseUploader{UserID: u.UserID, Name: u.Name})
			}
		}
	}

	h.renderPage(ctx, w, "render temp warehouses", pages.AdminTempWarehousesPage(data, lang, dir))
}

// buildTempWarehousesData runs the admin temp-warehouse listing and maps it into
// the page view model. Shared by the Super Admin and "my uploads" screens.
func (h *UIHandler) buildTempWarehousesData(ctx context.Context, filter compare.AdminTempWarehouseFilter, mineOnly bool, page, limit int) *pages.AdminTempWarehousesData {
	offset := (page - 1) * limit
	var rows []*compare.AdminTempWarehouse
	var totalCount int
	var totalRows int64
	var activeCount, archivedCount int

	if h.compareSvc != nil {
		var err error
		rows, totalCount, err = h.compareSvc.ListAdminTempWarehousesWithTotal(database.AsSystem(ctx), filter, limit, offset)
		if err != nil {
			h.log.ErrorContext(ctx, "list admin temp warehouses", "error", err)
		}
		totalRows, activeCount, archivedCount, err = h.compareSvc.AdminTempWarehouseStats(database.AsSystem(ctx), filter)
		if err != nil {
			h.log.ErrorContext(ctx, "stats admin temp warehouses", "error", err)
		}
	}

	items := make([]*pages.AdminTempWarehouseItem, 0, len(rows))
	for _, row := range rows {
		if row == nil || row.CompareFile == nil {
			continue
		}
		f := row.CompareFile
		source := "moderator"
		if f.OrganizationID != nil || !f.IsTempWarehouse {
			source = "vendor"
		}
		uid := f.UserID
		items = append(items, &pages.AdminTempWarehouseItem{
			ID:               f.ID,
			SupplierName:     f.SupplierName,
			OriginalFilename: f.OriginalFilename,
			RowCount:         f.RowCount,
			SizeBytes:        f.SizeBytes,
			Status:           string(f.Status),
			ArchiveReason:    f.ArchiveReason,
			CreatedBy:        &uid,
			CreatedAt:        f.CreatedAt,
			ArchivedAt:       f.ArchivedAt,
			UploaderName:     row.UploaderName,
			OrgName:          row.OrgName,
			SourceType:       source,
			Visibility:       f.Visibility,
		})
	}

	uploaderFilter := ""
	if filter.UploaderID != nil {
		uploaderFilter = strconv.FormatInt(*filter.UploaderID, 10)
	}

	var activeRunView *pages.TempWarehouseRunView
	if actor, ok := authctx.From(ctx); ok && actor.UserID > 0 {
		if run, payload := h.GetActiveTempWarehouseRun(ctx, actor.UserID); run != nil && payload != nil {
			activeRunView = &pages.TempWarehouseRunView{
				ID:            run.ID,
				PublicID:      run.PublicID,
				Filename:      run.Filename,
				State:         string(run.State),
				Phase:         run.Phase,
				Percent:       run.Percent,
				Step:          payload.Step,
				TotalFiles:    len(payload.FileIDs),
				CurrentFileID: payload.CurrentFileID,
				BaseURL:       payload.BaseURL,
				FileIDs:       payload.FileIDs,
				SupplierNames: payload.SupplierNames,
			}
		}
	}

	return &pages.AdminTempWarehousesData{
		Items:          items,
		TotalCount:     totalCount,
		TotalRows:      totalRows,
		ActiveCount:    activeCount,
		ArchivedCount:  archivedCount,
		Query:          filter.Search,
		StatusFilter:   statusFilterString(filter.Status),
		SourceFilter:   filter.Source,
		UploaderFilter: uploaderFilter,
		SortBy:         filter.SortBy,
		SortOrder:      filter.SortOrder,
		MineOnly:       mineOnly,
		Page:           page,
		PerPage:        limit,
		ActiveRun:      activeRunView,
	}
}

func statusFilterString(s *compare.CompareFileStatus) string {
	if s == nil {
		return ""
	}
	return string(*s)
}

// currentActorUserID returns the authenticated staff user's id, or 0.
func currentActorUserID(r *http.Request) int64 {
	if a, ok := authctx.From(r.Context()); ok {
		return a.UserID
	}
	return 0
}

// AdminTempWarehouseItemsJSON returns paginated items for a warehouse in JSON for the items modal.
func (h *UIHandler) AdminTempWarehouseItemsJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	fileID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || fileID <= 0 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "Invalid warehouse ID"})
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	rows, total, err := h.compareSvc.GetFileRowsPaginated(database.AsSystem(ctx), fileID, page, limit)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}

	type jsonRow struct {
		ID                 int64  `json:"id"`
		SKU                string `json:"sku"`
		RawName            string `json:"raw_name"`
		Price              string `json:"price"`
		Discount           string `json:"discount"`
		PriceAfterDiscount string `json:"price_after_discount"`
	}

	resRows := make([]jsonRow, 0, len(rows))
	for _, r := range rows {
		if r == nil {
			continue
		}
		resRows = append(resRows, jsonRow{
			ID:                 r.ID,
			SKU:                r.SKU,
			RawName:            r.RawName,
			Price:              r.Price.String(),
			Discount:           fmt.Sprintf("%.2f", r.Discount),
			PriceAfterDiscount: r.PriceAfterDiscount.String(),
		})
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	if totalPages < 1 {
		totalPages = 1
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":     true,
		"items":       resRows,
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": totalPages,
	})
}

// AdminTempWarehouseItemDeleteSubmit deletes a single product row from inside a warehouse.
func (h *UIHandler) AdminTempWarehouseItemDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	rowID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || rowID <= 0 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "Invalid item ID"})
		return
	}

	if err := h.compareSvc.DeleteFileRow(database.AsSystem(ctx), rowID); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

// AdminTempWarehouseDeleteSubmit deletes an entire temporary warehouse and all its rows.
func (h *UIHandler) AdminTempWarehouseDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	idStr := chi.URLParam(r, "id")
	fileID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || fileID <= 0 {
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", i18n.T(lang, "admin.temp_wh.invalid_id"))
		return
	}

	if err := h.compareSvc.DeleteFile(database.AsSystem(ctx), fileID); err != nil {
		h.log.ErrorContext(ctx, "delete temp warehouse", "error", err, "file_id", fileID)
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "success", i18n.T(lang, "admin.temp_wh.deleted_success"))
}

// AdminTempWarehouseToggleArchiveSubmit toggles warehouse archive status.
func (h *UIHandler) AdminTempWarehouseToggleArchiveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	idStr := chi.URLParam(r, "id")
	fileID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || fileID <= 0 {
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", i18n.T(lang, "admin.temp_wh.invalid_id"))
		return
	}

	f, err := h.compareSvc.GetFile(database.AsSystem(ctx), fileID)
	if err != nil || f == nil {
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", i18n.T(lang, "admin.temp_wh.not_found"))
		return
	}

	if f.Status == compare.FileArchived {
		if err := h.compareSvc.UnarchiveFile(database.AsSystem(ctx), fileID); err != nil {
			h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", h.safeMessage(err, lang))
			return
		}
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "success", i18n.T(lang, "admin.temp_wh.unarchived_success"))
	} else {
		if err := h.compareSvc.ArchiveFile(database.AsSystem(ctx), fileID, i18n.T(lang, "admin.temp_wh.manual_archive_reason")); err != nil {
			h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", h.safeMessage(err, lang))
			return
		}
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "success", i18n.T(lang, "admin.temp_wh.archived_success"))
	}
}

// AdminTempWarehouseExportXLSX streams all warehouse rows as an Excel file.
func (h *UIHandler) AdminTempWarehouseExportXLSX(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	fileID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || fileID <= 0 {
		http.Error(w, "Invalid warehouse ID", http.StatusBadRequest)
		return
	}

	fMeta, err := h.compareSvc.GetFile(database.AsSystem(ctx), fileID)
	if err != nil || fMeta == nil {
		http.Error(w, "Warehouse not found", http.StatusNotFound)
		return
	}

	rows, err := h.compareSvc.ListFileRows(database.AsSystem(ctx), fileID, 50000, 0)
	if err != nil {
		http.Error(w, "Failed to load warehouse items", http.StatusInternalServerError)
		return
	}

	f := excelize.NewFile()
	sheet := "Warehouse Items"
	f.SetSheetName("Sheet1", sheet)
	_ = f.SetSheetView(sheet, 0, &excelize.ViewOptions{
		RightToLeft: func(b bool) *bool { return &b }(true),
	})

	headers := []string{i18n.TDefault("w4_ui.sku_7"), i18n.TDefault("w4_ui.s_28_28"), i18n.TDefault("w4_ui.w4str_13_13"), i18n.TDefault("w4_ui.w4str_14_14"), i18n.TDefault("w4_ui.w4str_15_15")}
	for colIdx, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		_ = f.SetCellValue(sheet, cell, header)
	}

	for rowIdx, row := range rows {
		if row == nil {
			continue
		}
		rNum := rowIdx + 2
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", rNum), row.SKU)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", rNum), row.RawName)
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", rNum), row.Price.String())
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", rNum), fmt.Sprintf("%.2f%%", row.Discount))
		_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", rNum), row.PriceAfterDiscount.String())
	}

	safeName := strings.ReplaceAll(fMeta.SupplierName, " ", "_")
	filename := fmt.Sprintf("warehouse_%s_%s.xlsx", safeName, time.Now().Format("20060102"))

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	_ = f.Write(w)
}
