package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminFullErrorLogsPage renders system error logs with severity and status filters.
func (h *UIHandler) AdminFullErrorLogsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	statusFilter := r.URL.Query().Get("status")
	severityFilter := r.URL.Query().Get("severity")

	var logs []*platformadmin.ErrorLog
	var total int
	if h.adminSvc != nil {
		logs, total, _ = h.adminSvc.ListErrorLogs(database.AsSystem(ctx), platformadmin.ErrorLogFilter{
			Status: statusFilter,
			Level:  severityFilter,
			Limit:  limit,
			Offset: offset,
		})
	}

	h.renderPage(ctx, w, "render full error logs", pages.AdminFullErrorLogsPage(logs, total, statusFilter, severityFilter, lang, dir, page, limit))
}

// AdminFullErrorLogDetailPage renders single diagnostic error log details.
func (h *UIHandler) AdminFullErrorLogDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	logID, _ := strconv.ParseInt(idStr, 10, 64)

	var logEntry *platformadmin.ErrorLog
	if h.adminSvc != nil && logID > 0 {
		logEntry, _ = h.adminSvc.GetErrorLogByID(database.AsSystem(ctx), logID)
	}

	if logEntry == nil {
		http.Redirect(w, r, "/admin/full-error-logs", http.StatusSeeOther)
		return
	}

	h.renderPage(ctx, w, "render admin full error log detail", pages.AdminFullErrorLogDetailPage(logEntry, lang, dir))
}

// AdminFullActivityLogsPage renders system audit logs.
func (h *UIHandler) AdminFullActivityLogsPage(w http.ResponseWriter, r *http.Request) {
	h.AdminAuditPage(w, r)
}

// AdminFullActivityLogDetailPage renders single audit log entry.
func (h *UIHandler) AdminFullActivityLogDetailPage(w http.ResponseWriter, r *http.Request) {
	h.AdminAuditPage(w, r)
}

// AdminFullNotificationsPage renders admin notifications oversight.
func (h *UIHandler) AdminFullNotificationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/admin/notifications", http.StatusSeeOther)
		return
	}

	var logs []*notifications.NotificationLog
	unread := 0
	if h.notifSvc != nil {
		logs, _ = h.notifSvc.ListUserNotifications(ctx, userID, 50, 0)
		unread, _ = h.notifSvc.GetUnreadCount(ctx, userID)
	}

	h.renderPage(ctx, w, "render admin notifications", pages.Notifications(logs, unread, lang, dir, false))
}

// AdminSystemResourcesPage renders system resource availability and health.
func (h *UIHandler) AdminSystemResourcesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	systemName := chi.URLParam(r, "system")
	if systemName == "" {
		systemName = "all"
	}

	h.renderPage(ctx, w, "render admin system resources", pages.AdminSystemResourcesPage(systemName, lang, dir))
}

// AdminFirstLookPage renders admin onboarding introduction.
func (h *UIHandler) AdminFirstLookPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	h.renderPage(ctx, w, "render admin first look", pages.AdminFirstLookPage(lang, dir))
}

// AdminErrorLogTransitionSubmit updates an error status (INVESTIGATING, RESOLVED, IGNORED).
func (h *UIHandler) AdminErrorLogTransitionSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	logID, _ := strconv.ParseInt(idStr, 10, 64)
	newStatus := r.PostFormValue("status")

	if h.adminSvc != nil && logID > 0 && newStatus != "" {
		_ = h.adminSvc.UpdateErrorLogStatus(database.AsSystem(ctx), logID, newStatus)
	}

	http.Redirect(w, r, fmt.Sprintf("/admin/full-error-logs/%d", logID), http.StatusSeeOther)
}
