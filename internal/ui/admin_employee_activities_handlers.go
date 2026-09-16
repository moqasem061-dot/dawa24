package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminEmployeeActivitiesPage renders employee audit trail with rich filters, date range, and pagination.
func (h *UIHandler) AdminEmployeeActivitiesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	orgIDStr := strings.TrimSpace(r.URL.Query().Get("org_id"))
	userIDStr := strings.TrimSpace(r.URL.Query().Get("user_id"))
	dateFromStr := strings.TrimSpace(r.URL.Query().Get("date_from"))
	dateToStr := strings.TrimSpace(r.URL.Query().Get("date_to"))

	page := pagination.PageNumber(r)
	perPage := pagination.RowsPerPage(r)
	offset := (page - 1) * perPage

	var orgID *int64
	if o, err := strconv.ParseInt(orgIDStr, 10, 64); err == nil && o > 0 {
		orgID = &o
	}

	var userID *int64
	if u, err := strconv.ParseInt(userIDStr, 10, 64); err == nil && u > 0 {
		userID = &u
	}

	var dateFrom, dateTo *time.Time
	if t, err := time.Parse("2006-01-02", dateFromStr); err == nil {
		dateFrom = &t
	}
	if t, err := time.Parse("2006-01-02", dateToStr); err == nil {
		dateTo = &t
	}

	filter := platformadmin.AuditLogFilter{
		OrganizationID: orgID,
		ActorUserID:    userID,
		Action:         action,
		DateFrom:       dateFrom,
		DateTo:         dateTo,
		Search:         q,
		Limit:          perPage,
		Offset:         offset,
	}

	data := pages.AdminEmployeeActivitiesData{
		Page:           page,
		PerPage:        perPage,
		SearchQuery:    q,
		SelectedAction: action,
		DateFrom:       dateFromStr,
		DateTo:         dateToStr,
	}
	if orgID != nil {
		data.SelectedOrgID = *orgID
	}
	if userID != nil {
		data.SelectedUserID = *userID
	}

	if h.adminSvc != nil {
		entries, total, err := h.adminSvc.ListAuditLogWithFilter(ctx, filter)
		if err == nil {
			data.Entries = entries
			data.TotalCount = total
			data.TotalPages = (total + perPage - 1) / perPage
		}
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		orgs, _ := h.orgSvc.ListOrganizations(sysCtx, nil, nil, 100, 0)
		data.Organizations = orgs
	}

	if h.idSvc != nil {
		users, _ := h.idSvc.AdminListUsers(sysCtx, "", "")
		data.Users = users
	}

	h.renderPage(ctx, w, "render employee activities", pages.AdminEmployeeActivitiesPage(data, lang, dir))
}

// AdminEmployeeActivitiesExport streams filtered audit trail rows into an Excel (.xlsx) file.
func (h *UIHandler) AdminEmployeeActivitiesExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	orgIDStr := strings.TrimSpace(r.URL.Query().Get("org_id"))
	userIDStr := strings.TrimSpace(r.URL.Query().Get("user_id"))
	dateFromStr := strings.TrimSpace(r.URL.Query().Get("date_from"))
	dateToStr := strings.TrimSpace(r.URL.Query().Get("date_to"))

	var orgID *int64
	if o, err := strconv.ParseInt(orgIDStr, 10, 64); err == nil && o > 0 {
		orgID = &o
	}

	var userID *int64
	if u, err := strconv.ParseInt(userIDStr, 10, 64); err == nil && u > 0 {
		userID = &u
	}

	var dateFrom, dateTo *time.Time
	if t, err := time.Parse("2006-01-02", dateFromStr); err == nil {
		dateFrom = &t
	}
	if t, err := time.Parse("2006-01-02", dateToStr); err == nil {
		dateTo = &t
	}

	filter := platformadmin.AuditLogFilter{
		OrganizationID: orgID,
		ActorUserID:    userID,
		Action:         action,
		DateFrom:       dateFrom,
		DateTo:         dateTo,
		Search:         q,
		Limit:          10000,
		Offset:         0,
	}

	var entries []*platformadmin.AuditEntry
	if h.adminSvc != nil {
		entries, _, _ = h.adminSvc.ListAuditLogWithFilter(ctx, filter)
	}

	lang := langOf(r)
	f := excelize.NewFile()
	sheet := i18n.T(lang, "admin.audit.operations_sheet")
	f.SetSheetName("Sheet1", sheet)
	_ = f.SetSheetView(sheet, 0, &excelize.ViewOptions{
		RightToLeft: func(b bool) *bool { return &b }(true),
	})

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#0F172A"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})

	headers := []string{
		i18n.T(lang, "admin.audit.col_id"),
		i18n.T(lang, "admin.audit.col_date"),
		i18n.T(lang, "admin.audit.col_time"),
		i18n.T(lang, "admin.audit.col_actor"),
		i18n.T(lang, "admin.audit.col_email"),
		i18n.T(lang, "admin.audit.col_org"),
		i18n.T(lang, "admin.audit.col_org_code"),
		i18n.T(lang, "admin.audit.col_action_type"),
		i18n.T(lang, "admin.audit.col_action_code"),
		i18n.T(lang, "admin.audit.col_section_entity"),
		i18n.T(lang, "admin.audit.col_target_id"),
		i18n.T(lang, "admin.audit.col_ip"),
	}

	for colIdx, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		_ = f.SetCellValue(sheet, cell, header)
		_ = f.SetCellStyle(sheet, cell, cell, headerStyle)
	}

	for rowIdx, e := range entries {
		if e == nil {
			continue
		}
		rNum := rowIdx + 2

		orgIDDisplay := ""
		if e.OrganizationID != nil && *e.OrganizationID > 0 {
			orgIDDisplay = fmt.Sprintf("%d", *e.OrganizationID)
		}

		actionLabel := e.ActionLabelAr
		if label, ok := i18n.Lookup(i18n.Lang(lang), "audit.action."+e.Action); ok && label != "" {
			actionLabel = label
		}

		ipDisplay := e.IPAddress
		if ipDisplay == "" {
			ipDisplay = i18n.T(lang, "admin.audit.ip_internal")
		}

		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", rNum), e.ID)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", rNum), e.CreatedAt.Format("2006-01-02"))
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", rNum), e.CreatedAt.Format("03:04:05 PM"))
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", rNum), e.ActorName)
		_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", rNum), e.ActorEmail)
		_ = f.SetCellValue(sheet, fmt.Sprintf("F%d", rNum), e.OrganizationName)
		_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", rNum), orgIDDisplay)
		_ = f.SetCellValue(sheet, fmt.Sprintf("H%d", rNum), actionLabel)
		_ = f.SetCellValue(sheet, fmt.Sprintf("I%d", rNum), e.Action)
		_ = f.SetCellValue(sheet, fmt.Sprintf("J%d", rNum), e.EntityTypeAr)
		_ = f.SetCellValue(sheet, fmt.Sprintf("K%d", rNum), e.EntityID)
		_ = f.SetCellValue(sheet, fmt.Sprintf("L%d", rNum), ipDisplay)
	}

	filename := fmt.Sprintf("employee_activities_%s.xlsx", time.Now().Format("20060102_150405"))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	_ = f.Write(w)
}
