package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminSessionPlansPage renders session plan management and seats list.
func (h *UIHandler) AdminSessionPlansPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	h.renderPage(ctx, w, "render admin session plans", pages.AdminSessionPlansPage(lang, dir))
}

// AdminSessionPlanRequestsPage renders multi-session seat request queue.
func (h *UIHandler) AdminSessionPlanRequestsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	h.renderPage(ctx, w, "render admin session plan requests", pages.AdminSessionPlanRequestsPage(lang, dir))
}

// CustomerReportIssuePage renders issue report form for customers and vendors.
func (h *UIHandler) CustomerReportIssuePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var userIssues []*workflow.ReportIssue
	if actor, ok := authctx.From(ctx); ok && actor.UserID > 0 && h.wfSvc != nil {
		if list, err := h.wfSvc.ListIssuesByReporter(ctx, actor.UserID, 25, 0); err == nil {
			userIssues = list
		}
	}

	prefilledOrderID := strings.TrimSpace(r.URL.Query().Get("order_id"))
	h.renderPage(ctx, w, "render customer report issue", pages.CustomerReportIssuePage(lang, dir, userIssues, prefilledOrderID))
}

// CustomerReportIssueSubmit saves issue report into workflow.report_issues.
func (h *UIHandler) CustomerReportIssueSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/report-issue", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	issueType := strings.TrimSpace(r.FormValue("issue_type"))
	if issueType == "" {
		issueType = "technical"
	}
	priority := strings.TrimSpace(r.FormValue("priority"))
	if priority != "low" && priority != "high" {
		priority = "medium"
	}
	description := strings.TrimSpace(r.FormValue("description"))
	orderIDStr := strings.TrimSpace(r.FormValue("order_id"))

	if description == "" {
		h.redirectWithNotice(w, r, "/report-issue", "error", "يرجى كتابة تفاصيل البلاغ بشكل واضح.")
		return
	}

	if h.wfSvc == nil {
		h.redirectWithNotice(w, r, "/report-issue", "error", i18n.T(lang, "issues.service_unavailable"))
		return
	}

	var validOrderID *int64
	cleanOrderID := strings.TrimPrefix(strings.TrimSpace(orderIDStr), "#")
	cleanOrderID = strings.TrimSpace(cleanOrderID)
	if cleanOrderID != "" && h.commSvc != nil {
		sysCtx := database.AsSystem(ctx)
		// 1. Try resolving by Order Number (e.g. "ORD-2026-001")
		if ord, err := h.commSvc.GetOrderByNumber(sysCtx, cleanOrderID); err == nil && ord != nil {
			validOrderID = &ord.ID
		} else if oid, err := strconv.ParseInt(cleanOrderID, 10, 64); err == nil && oid > 0 {
			// 2. Try resolving by primary key ID
			if ord, err := h.commSvc.GetOrder(sysCtx, oid); err == nil && ord != nil {
				validOrderID = &ord.ID
			} else if ord, err := h.commSvc.GetOrderByNumber(sysCtx, fmt.Sprintf("%d", oid)); err == nil && ord != nil {
				validOrderID = &ord.ID
			}
		}
	}

	issue := &workflow.ReportIssue{
		ReportedBy:  actor.UserID,
		IssueType:   issueType,
		Description: description,
		Priority:    priority,
	}
	if actor.OrganizationID > 0 {
		issue.OrganizationID = &actor.OrganizationID
	}
	if validOrderID != nil {
		issue.OrderID = validOrderID
	} else if cleanOrderID != "" {
		// Preserve user's provided order/shipment reference safely in description
		// without violating database foreign key constraint (SQLSTATE 23503).
		issue.Description = fmt.Sprintf("%s\n\n[رقم الطلب/الشحنة المرفق من المستخدم: %s]", issue.Description, orderIDStr)
	}

	created, err := h.wfSvc.ReportIssue(ctx, issue)
	if err != nil {
		h.log.ErrorContext(ctx, "failed reporting issue", "error", err, "user_id", actor.UserID)
		h.redirectWithNotice(w, r, "/report-issue", "error", h.safeMessage(err, lang))
		return
	}

	// Dispatch in-app notification to administrators
	reporterName := h.resolveUserName(ctx, actor.UserID)
	orgName := ""
	if actor.OrganizationID > 0 {
		orgName = h.resolveOrgName(ctx, actor.OrganizationID)
	}
	h.notifyAdminNewIssue(ctx, created, reporterName, orgName)

	h.redirectWithNotice(w, r, "/report-issue", "success", "تم إرسال البلاغ بنجاح وسيتم متابعته من قبل فريق الدعم الفني.")
}

// AdminReportIssuesPage renders admin review queue for submitted user issues.
func (h *UIHandler) AdminReportIssuesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	issueType := strings.TrimSpace(r.URL.Query().Get("type"))
	priority := strings.TrimSpace(r.URL.Query().Get("priority"))
	search := strings.TrimSpace(r.URL.Query().Get("q"))

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize := 20
	offset := (page - 1) * pageSize

	var issues []*workflow.ReportIssueDetail
	var total int
	var stats *workflow.ReportIssueStats

	if h.wfSvc != nil {
		var err error
		filter := workflow.ReportIssueFilter{
			Status:    status,
			IssueType: issueType,
			Priority:  priority,
			Search:    search,
			Limit:     pageSize,
			Offset:    offset,
		}
		issues, total, stats, err = h.wfSvc.ListIssuesAdmin(ctx, filter)
		if err != nil {
			h.log.ErrorContext(ctx, "failed listing report issues for admin", "error", err)
		}
	}

	totalPages := 1
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}

	data := pages.AdminReportIssuesData{
		Issues:      issues,
		Total:       total,
		Stats:       stats,
		Status:      status,
		IssueType:   issueType,
		Priority:    priority,
		Search:      search,
		CurrentPage: page,
		TotalPages:  totalPages,
		PageSize:    pageSize,
	}

	h.renderPage(ctx, w, "render admin report issues", pages.AdminReportIssuesPage(lang, dir, data))
}

// AdminReportIssueUpdateSubmit handles updating issue status and adding response notes.
func (h *UIHandler) AdminReportIssueUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/report-issues", "error", "معرف البلاغ غير صالح.")
		return
	}

	_ = r.ParseForm()
	status := strings.TrimSpace(r.FormValue("status"))
	responseNotes := strings.TrimSpace(r.FormValue("response_notes"))

	if msg := h.updateReportedIssue(ctx, id, status, responseNotes); msg != "" {
		h.redirectWithNotice(w, r, "/admin/report-issues", "error", msg)
		return
	}

	h.redirectWithNotice(w, r, "/admin/report-issues", "success", "تم تحديث حالة البلاغ بنجاح وإشعار المستخدم بالرد.")
}

// updateReportedIssue sets an issue's status and response and notifies the
// reporter. It returns a user-facing message on failure.
func (h *UIHandler) updateReportedIssue(ctx context.Context, id int64, status, responseNotes string) string {
	if status != "pending" && status != "in_progress" && status != "resolved" {
		return "حالة البلاغ المحددة غير صحيحة."
	}
	if h.wfSvc == nil {
		return "خدمة البلاغات غير متاحة حالياً."
	}
	existing, err := h.wfSvc.GetIssueByID(ctx, id)
	if err != nil || existing == nil {
		h.log.ErrorContext(ctx, "failed retrieving issue for update", "error", err, "id", id)
		return "لم يتم العثور على البلاغ المطلوب."
	}
	if err := h.wfSvc.UpdateIssueStatus(ctx, id, status, responseNotes); err != nil {
		h.log.ErrorContext(ctx, "failed updating issue status", "error", err, "id", id)
		return "حدث خطأ أثناء تحديث حالة البلاغ."
	}
	h.notifyUserIssueResponse(ctx, existing, status, responseNotes)
	return ""
}
