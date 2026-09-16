package ui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminOrgImportSavingUploadPage renders the savings products import upload screen
// for an admin acting on behalf of a specific target organization.
func (h *UIHandler) AdminOrgImportSavingUploadPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	if orgID <= 0 {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "validation.invalid_id"))
		return
	}

	sysCtx := database.AsSystem(ctx)
	targetOrgName, targetOrgType := h.resolveTargetOrgInfo(sysCtx, orgID)
	sessions := globalSavingImportSessionStore.ListSessions(orgID)

	view := pages.SavingImportView{
		AIAvailable:         h.matchEnhancer != nil,
		AIUnavailableReason: savingAIUnavailableReason(h.matchEnhancer, lang),
		Audience:            "admin",
		TargetOrgName:       targetOrgName,
		TargetOrgType:       targetOrgType,
		BaseURL:             "/admin/organizations/import",
		ImportURL:           fmt.Sprintf("/admin/organizations/import/%d/saving", orgID),
		Sessions:            sessions,
		NoticeType:          r.URL.Query().Get("notice_type"),
		NoticeMsg:           r.URL.Query().Get("notice"),
	}

	h.renderPage(ctx, w, "render admin org saving upload page", pages.SavingImportPage(view, lang, dir))
}

// AdminOrgImportSavingsUploadSubmit starts an import session for the selected target organization.
func (h *UIHandler) AdminOrgImportSavingsUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)

	targetOrgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	if targetOrgID <= 0 {
		targetOrgID, _ = strconv.ParseInt(r.PostFormValue("org_id"), 10, 64)
	}
	if targetOrgID <= 0 {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "validation.invalid_id"))
		return
	}

	if err := parseImportUpload(w, r); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/saving", targetOrgID), "error", i18n.T(lang, "customer.saving.import.file_too_large_short"))
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/saving", targetOrgID), "error", i18n.T(lang, "customer.saving.import.select_file"))
		return
	}
	defer file.Close()

	if !SupportedUploadName(fileHeader.Filename) {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/saving", targetOrgID), "error", unsupportedUploadMsg(lang))
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/saving", targetOrgID), "error", i18n.T(lang, "customer.saving.import.file_empty"))
		return
	}

	if err := filesecurity.ValidateSpreadsheetSecurity(fileBytes, fileHeader.Filename); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/saving", targetOrgID), "error", filesecurity.SecurityErrorMessage)
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, fileHeader.Filename)
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", fileHeader.Filename)
		msg := i18n.T(lang, "customer.saving.import.parse_error_short")
		if err != nil {
			msg = h.safeMessage(err, lang)
		}
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/saving", targetOrgID), "error", msg)
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
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/saving", targetOrgID), "error", i18n.T(lang, "customer.saving.import.no_data_rows"))
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
		targetOrgID,
		actor.UserID,
		fileHeader.Filename,
		headers,
		sampleRows,
		dataRows,
		SavingDetectedCols{
			NameCol:      nameCol,
			SKUCol:       skuCol,
			QtyCol:       qtyCol,
			PriceCol:     priceCol,
			ProductIDCol: productIDCol,
		},
	)

	h.log.InfoContext(ctx, "admin initialized saving products import session",
		"session_id", session.ID,
		"admin_user_id", actor.UserID,
		"target_org_id", targetOrgID,
		"rows", len(dataRows),
	)

	http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/%d/runs/%s/mapping", targetOrgID, session.ID), http.StatusSeeOther)
}

func (h *UIHandler) resolveTargetOrgInfo(ctx context.Context, orgID int64) (string, string) {
	if h.orgSvc == nil || orgID <= 0 {
		return "", ""
	}
	targetOrg, err := h.orgSvc.GetOrganization(ctx, orgID)
	if err != nil || targetOrg == nil {
		return "", ""
	}
	name := targetOrg.TradeName.Get(i18n.AR)
	if name == "" {
		name = targetOrg.TradeName.Get(i18n.EN)
	}
	if name == "" {
		name = targetOrg.LegalName
	}
	return name, string(targetOrg.Type)
}
