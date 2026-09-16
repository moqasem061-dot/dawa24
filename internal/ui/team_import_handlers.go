package ui

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) handleTeamImportPage(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=/%s/team/import", audience), http.StatusSeeOther)
		return
	}

	sessions := globalTeamImportSessionStore.ListSessions(actor.OrganizationID)
	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("msg")
	}

	view := pages.TeamImportView{
		Audience:   audience,
		BaseURL:    fmt.Sprintf("/%s/team", audience),
		ImportURL:  fmt.Sprintf("/%s/team/import", audience),
		Sessions:   sessions,
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
	}

	h.renderPage(ctx, w, fmt.Sprintf("render %s team import", audience), pages.TeamImportPage(view, lang, dir))
}

func (h *UIHandler) handleTeamImportUploadSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=/%s/team/import", audience), http.StatusSeeOther)
		return
	}

	baseImportURL := fmt.Sprintf("/%s/team/import", audience)

	if err := parseImportUpload(w, r); err != nil {
		h.redirectWithNotice(w, r, baseImportURL, "error", i18n.T(langOf(r), "customer.saving.import.file_too_large_short"))
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, baseImportURL, "error", i18n.T(langOf(r), "customer.saving.import.select_file"))
		return
	}
	defer file.Close()

	if !SupportedUploadName(fileHeader.Filename) {
		h.redirectWithNotice(w, r, baseImportURL, "error", unsupportedUploadMsg(langOf(r)))
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, baseImportURL, "error", i18n.T(langOf(r), "customer.saving.import.file_empty"))
		return
	}

	if err := filesecurity.ValidateSpreadsheetSecurity(fileBytes, fileHeader.Filename, filesecurity.WithAllowEmails(true)); err != nil {
		h.redirectWithNotice(w, r, baseImportURL, "error", filesecurity.SecurityErrorMessage)
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, fileHeader.Filename, sheet.WithAllowEmails(true))
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", fileHeader.Filename)
		msg := i18n.T(langOf(r), "customer.saving.import.parse_error_short")
		if err != nil && strings.Contains(err.Error(), filesecurity.SecurityErrorMessage) {
			msg = filesecurity.SecurityErrorMessage
		}
		h.redirectWithNotice(w, r, baseImportURL, "error", msg)
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
		h.redirectWithNotice(w, r, baseImportURL, "error", i18n.T(langOf(r), "customer.team.import.no_employee_rows"))
		return
	}

	var sampleRows [][]string
	limit := 5
	if len(dataRows) < limit {
		limit = len(dataRows)
	}
	sampleRows = dataRows[:limit]

	detectedCols := DetectTeamColumns(headers, sampleRows)

	// Fetch company roles
	h.ensureCompanyRoles(ctx, actor.OrganizationID, actor.OrgType)
	var companyRoles []pages.TeamRoleOption
	if h.orgSvc != nil {
		if roles, err := h.orgSvc.ListRoles(ctx, actor.OrganizationID); err == nil {
			for _, role := range roles {
				if role == nil || role.IsOwner || role.Key == "org_owner" {
					continue
				}
				companyRoles = append(companyRoles, pages.TeamRoleOption{
					ID:      role.ID,
					Key:     role.Key,
					Name:    role.Name.Get(i18n.AR),
					IsOwner: role.IsOwner,
				})
			}
		}
	}

	// Fetch branches
	var branches []pages.TeamBranchOption
	if h.orgSvc != nil {
		if branchList, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil {
			for _, b := range branchList {
				branches = append(branches, pages.TeamBranchOption{
					ID:   b.ID,
					Name: b.Name.Get(i18n.AR),
					Code: b.Code,
				})
			}
		}
	}

	excelRoles := extractUniqueExcelRoles(dataRows, detectedCols.RoleCol, companyRoles)

	var defaultRoleID int64
	for _, cr := range companyRoles {
		if !cr.IsOwner && cr.Key != "org_owner" {
			defaultRoleID = cr.ID
			break
		}
	}
	if defaultRoleID == 0 && len(companyRoles) > 0 {
		defaultRoleID = companyRoles[0].ID
	}

	session := globalTeamImportSessionStore.NewSession(actor.OrganizationID, actor.UserID, audience, fileHeader.Filename, len(dataRows))
	session.Phase = pages.TeamPhaseMapping
	session.Headers = headers
	session.SampleRows = sampleRows
	session.RawDataRows = dataRows
	session.DetectedCols = detectedCols
	session.ExcelRoles = excelRoles
	session.CompanyRoles = companyRoles
	session.Branches = branches
	session.DefaultRoleID = defaultRoleID

	http.Redirect(w, r, fmt.Sprintf("/%s/team/import/%s", audience, session.ID), http.StatusSeeOther)
}

func (h *UIHandler) handleTeamImportSampleDownload(w http.ResponseWriter, audience, filename string) {
	bytes, err := GenerateTeamSampleExcel(audience)
	if err != nil {
		http.Error(w, "Failed to generate sample spreadsheet", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(bytes)))
	_, _ = w.Write(bytes)
}

func (h *UIHandler) handleTeamImportSessionPage(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=/%s/team/import", audience), http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalTeamImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		h.redirectWithNotice(w, r, fmt.Sprintf("/%s/team/import", audience), "error", i18n.T(langOf(r), "customer.saving.import.session_not_found"))
		return
	}

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("msg")
	}

	view := pages.TeamImportView{
		Audience:   audience,
		BaseURL:    fmt.Sprintf("/%s/team", audience),
		ImportURL:  fmt.Sprintf("/%s/team/import", audience),
		Session:    session,
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
	}

	h.renderPage(ctx, w, fmt.Sprintf("render %s team import session", audience), pages.TeamImportPage(view, lang, dir))
}

