package ui

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminProductImagesImportPage renders the image recovery main page with dropzone & past sessions.
func (h *UIHandler) AdminProductImagesImportPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/products/images/import", http.StatusSeeOther)
		return
	}

	sessions := globalAdminImageImportSessionStore.ListSessions()
	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	view := pages.AdminProductImagesImportView{
		Session:    nil,
		Sessions:   sessions,
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
		Actor:      actor,
	}

	h.renderPage(ctx, w, "render admin product images import page", pages.AdminProductImagesImportPage(view, lang, dir))
}

// AdminProductImagesUploadSubmit handles spreadsheet file upload and opens a new image recovery session.
func (h *UIHandler) AdminProductImagesUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/products/images/import", http.StatusSeeOther)
		return
	}

	if err := parseImportUpload(w, r); err != nil {
		h.redirectWithNotice(w, r, "/admin/products/images/import", "error", i18n.T(lang, "admin.image_import.file_too_large"))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/admin/products/images/import", "error", i18n.T(lang, "admin.image_import.select_valid_file"))
		return
	}
	defer file.Close()

	// The extension is checked to catch an obvious mistake early — a PDF, an
	// image — and nothing more. What the file actually IS, is decided by its
	// bytes: suppliers and admins rename files freely, and a .xls that is
	// really an HTML table or a .xlsx that is really a CSV are both ordinary
	// here. The shared reader sniffs the real container.
	ext := strings.ToLower(filepath.Ext(header.Filename))
	switch ext {
	case ".xlsx", ".xls", ".xlsm", ".csv", ".txt", ".tsv", ".htm", ".html", ".xml", "":
	default:
		h.redirectWithNotice(w, r, "/admin/products/images/import", "error",
			i18n.T(lang, "admin.image_import.unsupported_format"))
		return
	}

	data, err := io.ReadAll(file)
	if err != nil || len(data) == 0 {
		h.redirectWithNotice(w, r, "/admin/products/images/import", "error", i18n.T(lang, "admin.image_import.file_empty"))
		return
	}

	// This file is a list of image addresses; that is what the feature is. The
	// URLs it carries are fetched by the importer under its own scheme, host
	// and size rules, which is where that check belongs.
	rawRows, err := sheet.ReadRows(data, header.Filename, sheet.WithAllowURLs(true))
	if err != nil {
		// The reader's own message names the actual problem — a password, a
		// damaged BIFF workbook, an unrecognised container — and is far more
		// use than "no valid data".
		h.redirectWithNotice(w, r, "/admin/products/images/import", "error", err.Error())
		return
	}
	if len(rawRows) < 2 {
		h.redirectWithNotice(w, r, "/admin/products/images/import", "error",
			i18n.T(lang, "admin.image_import.header_only"))
		return
	}

	headers := rawRows[0]
	dataRows := rawRows[1:]

	skuCol, urlCol := detectImageImportColumns(headers)

	sampleCount := len(dataRows)
	if sampleCount > 5 {
		sampleCount = 5
	}
	sampleRows := make([][]string, sampleCount)
	copy(sampleRows, dataRows[:sampleCount])

	session := globalAdminImageImportSessionStore.NewSession(actor.OrganizationID, actor.UserID, header.Filename, len(dataRows))
	session.Headers = headers
	session.DetectedSKUCol = skuCol
	session.DetectedURLCol = urlCol
	session.SampleRows = sampleRows
	session.RawDataRows = dataRows

	http.Redirect(w, r, fmt.Sprintf("/admin/products/images/import/%s", session.ID), http.StatusSeeOther)
}

func detectImageImportColumns(headers []string) (skuCol, urlCol int) {
	skuCol = -1
	urlCol = -1

	for idx, rawH := range headers {
		h := strings.ToLower(strings.TrimSpace(rawH))
		if skuCol == -1 && (strings.Contains(h, "sku") || strings.Contains(h, i18n.TDefault("w4_ui.s_2_2")) || strings.Contains(h, i18n.TDefault("w4_ui.s_3_3")) || strings.Contains(h, "barcode") || strings.Contains(h, i18n.TDefault("w4_ui.s_4_4")) || strings.Contains(h, "code")) {
			skuCol = idx
		}
		if urlCol == -1 && (strings.Contains(h, "url") || strings.Contains(h, "image") || strings.Contains(h, i18n.TDefault("w4_ui.s_5_5")) || strings.Contains(h, "link") || strings.Contains(h, i18n.TDefault("w4_ui.s_6_6")) || strings.Contains(h, "photo") || strings.Contains(h, "img")) {
			urlCol = idx
		}
	}

	if skuCol == -1 && len(headers) > 0 {
		skuCol = 0
	}
	if urlCol == -1 && len(headers) > 1 {
		urlCol = 1
	}

	return skuCol, urlCol
}

// AdminProductImagesSessionPage displays the wizard stage for mapping, progress, or results review.
func (h *UIHandler) AdminProductImagesSessionPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/products/images/import", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalAdminImageImportSessionStore.GetSession(sessionID)
	if !ok {
		h.redirectWithNotice(w, r, "/admin/products/images/import", "error", i18n.T(lang, "admin.image_import.session_not_found"))
		return
	}

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	view := pages.AdminProductImagesImportView{
		Session:    session,
		Sessions:   globalAdminImageImportSessionStore.ListSessions(),
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
		Actor:      actor,
	}

	h.renderPage(ctx, w, "render admin product images session page", pages.AdminProductImagesImportPage(view, lang, dir))
}

// AdminProductImagesMappingSubmit confirms column selection and launches image downloading in the background.
func (h *UIHandler) AdminProductImagesMappingSubmit(w http.ResponseWriter, r *http.Request) {
	lang := langOf(r)
	sessionID := chi.URLParam(r, "id")
	session, ok := globalAdminImageImportSessionStore.GetSession(sessionID)
	if !ok {
		h.redirectWithNotice(w, r, "/admin/products/images/import", "error", i18n.T(lang, "admin.image_import.session_not_found"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/products/images/import/%s", sessionID), "error", h.safeMessage(err, lang))
		return
	}

	skuCol, _ := strconv.Atoi(r.PostFormValue("sku_col"))
	urlCol, _ := strconv.Atoi(r.PostFormValue("url_col"))

	if skuCol < 0 || urlCol < 0 || skuCol == urlCol {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/products/images/import/%s", sessionID), "error", i18n.T(lang, "admin.image_import.select_columns_required"))
		return
	}

	idType := strings.ToLower(strings.TrimSpace(r.PostFormValue("identifier_type")))
	if idType != "barcode" {
		idType = "sku"
	}
	session.IdentifierType = idType
	session.DetectedSKUCol = skuCol
	session.DetectedURLCol = urlCol

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				h.log.Error("admin image import processing panicked", "session_id", sessionID, "panic", rec)
			}
		}()
		bgCtx := context.Background()
		globalAdminImageImportSessionStore.ProcessImageImport(bgCtx, sessionID, skuCol, urlCol, h.catSvc, h.storage)
	}()

	http.Redirect(w, r, fmt.Sprintf("/admin/products/images/import/%s", sessionID), http.StatusSeeOther)
}

// AdminProductImagesProgressJSON returns the live execution state and stats for real-time progress bar.
func (h *UIHandler) AdminProductImagesProgressJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	sessionID := chi.URLParam(r, "id")
	session, ok := globalAdminImageImportSessionStore.GetSession(sessionID)
	if !ok {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "admin.image_import.session_not_found")})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":        true,
		"phase":          session.Phase,
		"progress":       session.Progress,
		"progress_note":  session.ProgressNote,
		"total_rows":     session.TotalRows,
		"success_rows":   session.SuccessRows,
		"not_found_rows": session.NotFoundRows,
		"error_rows":     session.ErrorRows,
	})
}

// AdminProductImagesCancelSubmit cancels or deletes an import session.
func (h *UIHandler) AdminProductImagesCancelSubmit(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	globalAdminImageImportSessionStore.DeleteSession(sessionID)
	h.redirectWithNotice(w, r, "/admin/products/images/import", "success", i18n.T(langOf(r), "admin.image_import.session_deleted_success"))
}

// AdminProductImagesSampleXLSX streams download of a clean Excel template.
func (h *UIHandler) AdminProductImagesSampleXLSX(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()
	sheet := "Sheet1"
	f.SetSheetName("Sheet1", sheet)
	_ = f.SetSheetView(sheet, 0, &excelize.ViewOptions{RightToLeft: boolPtr(true)})

	headers := []string{i18n.TDefault("w4_ui.sku_7"), i18n.TDefault("w4_ui.image_url_8")}
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#0284C7"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	_ = f.SetRowHeight(sheet, 1, 26)

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
		_ = f.SetCellStyle(sheet, cell, cell, headerStyle)
	}

	rows := [][]string{
		{"PAN-500-01", "https://example.com/images/panadol-extra.jpg"},
		{"CAT-050-02", "https://example.com/images/cataflam-50mg.jpg"},
		{"AUG-100-03", "https://example.com/images/augmentin-1g.jpg"},
	}
	for rIdx, row := range rows {
		for cIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(cIdx+1, rIdx+2)
			_ = f.SetCellValue(sheet, cell, val)
		}
	}

	_ = f.SetColWidth(sheet, "A", "A", 22)
	_ = f.SetColWidth(sheet, "B", "B", 65)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=product_images_template.xlsx")
	_ = f.Write(w)
}

// AdminProductImagesSampleCSV streams download of a clean CSV template.
func (h *UIHandler) AdminProductImagesSampleCSV(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF") // UTF-8 BOM
	cw := csv.NewWriter(&buf)
	_ = cw.Write([]string{i18n.TDefault("w4_ui.sku_7"), i18n.TDefault("w4_ui.image_url_8")})
	_ = cw.Write([]string{"PAN-500-01", "https://example.com/images/panadol-extra.jpg"})
	_ = cw.Write([]string{"CAT-050-02", "https://example.com/images/cataflam-50mg.jpg"})
	_ = cw.Write([]string{"AUG-100-03", "https://example.com/images/augmentin-1g.jpg"})
	cw.Flush()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=product_images_template.csv")
	_, _ = w.Write(buf.Bytes())
}
