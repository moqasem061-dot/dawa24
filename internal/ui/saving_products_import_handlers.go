package ui

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// handleSavingProductsImportPage renders the sessions list and file dropzone for customer or vendor.
func (h *UIHandler) handleSavingProductsImportPage(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	targetImportURL := fmt.Sprintf("/%s/saving-products/import", audience)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetImportURL), http.StatusSeeOther)
		return
	}

	sessions := globalSavingImportSessionStore.ListSessions(actor.OrganizationID)
	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	view := pages.SavingImportView{
		AIAvailable:         h.matchEnhancer != nil,
		AIUnavailableReason: savingAIUnavailableReason(h.matchEnhancer, lang),
		Audience:            audience,
		BaseURL:             fmt.Sprintf("/%s/saving-products", audience),
		ImportURL:           targetImportURL,
		Sessions:            sessions,
		NoticeType:          noticeType,
		NoticeMsg:           noticeMsg,
	}

	h.renderPage(ctx, w, fmt.Sprintf("render %s saving import page", audience), pages.SavingImportPage(view, lang, dir))
}

// handleSavingProductsImportUploadSubmit handles file upload and creates new import session for customer or vendor.
func (h *UIHandler) handleSavingProductsImportUploadSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	targetImportURL := fmt.Sprintf("/%s/saving-products/import", audience)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetImportURL), http.StatusSeeOther)
		return
	}

	if err := parseImportUpload(w, r); err != nil {
		h.redirectWithNotice(w, r, targetImportURL, "error", i18n.T(langOf(r), "customer.saving.import.file_too_large_short"))
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, targetImportURL, "error", i18n.T(langOf(r), "customer.saving.import.select_file"))
		return
	}
	defer file.Close()

	if !SupportedUploadName(fileHeader.Filename) {
		h.redirectWithNotice(w, r, targetImportURL, "error", unsupportedUploadMsg(langOf(r)))
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, targetImportURL, "error", i18n.T(langOf(r), "customer.saving.import.file_empty"))
		return
	}

	if err := filesecurity.ValidateSpreadsheetSecurity(fileBytes, fileHeader.Filename); err != nil {
		h.redirectWithNotice(w, r, targetImportURL, "error", filesecurity.SecurityErrorMessage)
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, fileHeader.Filename)
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", fileHeader.Filename)
		msg := i18n.T(langOf(r), "customer.saving.import.parse_error_short")
		if err != nil && strings.Contains(err.Error(), filesecurity.SecurityErrorMessage) {
			msg = filesecurity.SecurityErrorMessage
		}
		h.redirectWithNotice(w, r, targetImportURL, "error", msg)
		return
	}

	layout, _ := productmatch.AnalyzeLayout(rawRows)
	var headers []string
	dataStart := 1
	if layout.HeaderRow >= 0 && layout.HeaderRow < len(rawRows) {
		headers = layout.Headers
		dataStart = layout.FirstDataRow
	} else if len(rawRows) > 0 {
		headers = rawRows[0]
		dataStart = 1
	}

	if dataStart > len(rawRows) {
		dataStart = len(rawRows)
	}

	dataRows := rawRows[dataStart:]
	if len(dataRows) == 0 {
		h.redirectWithNotice(w, r, targetImportURL, "error", i18n.T(langOf(r), "customer.saving.import.no_data_rows"))
		return
	}

	var sampleRows [][]string
	limit := 5
	if len(dataRows) < limit {
		limit = len(dataRows)
	}
	sampleRows = dataRows[:limit]

	nameCol, skuCol, qtyCol, priceCol, productIDCol := detectSavingProductColumns(
		headers,
		sampleRows,
		"", "", "", "", "",
	)

	session := globalSavingImportSessionStore.NewMappingSession(
		actor.OrganizationID, actor.UserID, fileHeader.Filename,
		headers, sampleRows, dataRows,
		SavingDetectedCols{
			NameCol:      nameCol,
			SKUCol:       skuCol,
			QtyCol:       qtyCol,
			PriceCol:     priceCol,
			ProductIDCol: productIDCol,
		},
	)

	http.Redirect(w, r, fmt.Sprintf("/%s/saving-products/import/%s", audience, session.ID), http.StatusSeeOther)
}

// handleSavingProductsImportSessionPage renders the session at its current wizard phase for customer or vendor.
func (h *UIHandler) handleSavingProductsImportSessionPage(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	targetImportURL := fmt.Sprintf("/%s/saving-products/import", audience)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetImportURL), http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		h.redirectWithNotice(w, r, targetImportURL, "error", i18n.T(langOf(r), "customer.saving.import.session_not_found"))
		return
	}

	matchFilter := strings.TrimSpace(r.URL.Query().Get("match"))
	sortBy := strings.TrimSpace(r.URL.Query().Get("sort"))
	sortOrder := strings.TrimSpace(r.URL.Query().Get("order"))
	search := strings.TrimSpace(r.URL.Query().Get("q"))

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)

	filter := SavingRowFilter{
		Search:      search,
		MatchFilter: matchFilter,
		SortBy:      sortBy,
		SortOrder:   sortOrder,
		Page:        page,
		Limit:       limit,
	}

	rows, total := globalSavingImportSessionStore.FilterItems(session, filter)

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	view := pages.SavingImportView{
		AIAvailable:         h.matchEnhancer != nil,
		AIUnavailableReason: savingAIUnavailableReason(h.matchEnhancer, lang),
		Audience:            audience,
		BaseURL:             fmt.Sprintf("/%s/saving-products", audience),
		ImportURL:           targetImportURL,
		Session:             session,
		Filter:              filter,
		Rows:                rows,
		RowTotal:            total,
		NoticeType:          noticeType,
		NoticeMsg:           noticeMsg,
	}

	h.renderPage(ctx, w, fmt.Sprintf("render %s saving import session page", audience), pages.SavingImportPage(view, lang, dir))
}

// handleSavingProductsImportMapSubmit processes column mapping and performs matching for customer or vendor.
func (h *UIHandler) handleSavingProductsImportMapSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	lang := langOf(r)
	targetImportURL := fmt.Sprintf("/%s/saving-products/import", audience)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetImportURL), http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		h.redirectWithNotice(w, r, targetImportURL, "error", i18n.T(lang, "saving.import.session_not_found"))
		return
	}

	var invalidDataMsg string
	if audience == "vendor" {
		invalidDataMsg = i18n.T(lang, "common.invalid_form_data")
	} else {
		invalidDataMsg = i18n.T(lang, "validation.invalid_data")
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/%s/saving-products/import/%s", audience, sessionID), "error", invalidDataMsg)
		return
	}

	colName := strings.TrimSpace(r.FormValue("col_name"))
	colSKU := strings.TrimSpace(r.FormValue("col_sku"))
	colQty := strings.TrimSpace(r.FormValue("col_qty"))
	colPrice := strings.TrimSpace(r.FormValue("col_price"))
	colProductID := strings.TrimSpace(r.FormValue("col_product_id"))
	matchChoice := ParseMatchChoice(r)
	useAI := ParseUseAIFromWizard(r)

	// The mapping screen names columns by their header text, so a file with two
	// identically headed columns would bind the later one. headerColumn takes
	// the first, which is the one the preview showed.
	headerColumn := func(want string) int {
		if want == "" {
			return -1
		}
		for idx, hName := range session.Headers {
			if hName == want {
				return idx
			}
		}
		return -1
	}

	nCol := headerColumn(colName)
	sCol := headerColumn(colSKU)
	qCol := headerColumn(colQty)
	pCol := headerColumn(colPrice)

	// The دوا 24 product-id column.
	//
	// It used to be hard-coded to -1 here while the drag-and-drop path honoured
	// it, so the same file matched differently depending on which screen
	// uploaded it — and the "دوا 24 كود الصنف فقط" strategy this form offers
	// could never match a single row, because the column it reads was never
	// passed. The screen now carries the select, and an unanswered one falls
	// back to what the parser detected rather than to nothing.
	pidCol := headerColumn(colProductID)
	if colProductID == "" && pidCol < 0 {
		pidCol = session.DetectedCols.ProductIDCol
	}
	if pidCol >= len(session.Headers) {
		pidCol = -1
	}

	publicID, _, startErr := h.startSavingImportRun(
		ctx, actor, session.Filename,
		append([][]string{session.Headers}, session.RawDataRows...),
		session.Headers, session.SampleRows,
		nCol, sCol, qCol, pCol, pidCol,
		matchChoice, useAI, langOf(r), audience,
	)
	if startErr != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/%s/saving-products/import/%s", audience, sessionID), "error", h.safeMessage(startErr, langOf(r)))
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/%s/saving-products/import/%s", audience, publicID), http.StatusSeeOther)
}

func isAllDigitsOrCode(s string) bool {
	if s == "" {
		return false
	}
	digits := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits++
		} else if r != '-' && r != '_' && r != '.' && r != '/' && r != ' ' {
			return false
		}
	}
	return digits > 0
}

func isDescriptiveArabicText(s string) bool {
	if len(s) < 3 {
		return false
	}
	hasArabic := false
	hasSpace := false
	for _, r := range s {
		if (r >= 0x0600 && r <= 0x06FF) || (r >= 0x0750 && r <= 0x077F) {
			hasArabic = true
		}
		if r == ' ' {
			hasSpace = true
		}
	}
	return hasArabic && hasSpace
}
