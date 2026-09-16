package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) NotificationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/notifications", http.StatusSeeOther)
		return
	}

	if h.notifSvc == nil {
		h.renderPage(ctx, w, "render notifications page", pages.Notifications(nil, 0, lang, dir, h.isHTMX(r)))
		return
	}

	logs, err := h.notifSvc.ListUserNotifications(ctx, userID, h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	if actor, ok := authctx.From(ctx); ok {
		logs = filterNotificationsForActor(actor, logs)
	}

	unread, _ := h.notifSvc.GetUnreadCount(ctx, userID)

	h.renderPage(ctx, w, "render notifications page", pages.Notifications(logs, unread, lang, dir, h.isHTMX(r)))
}

func (h *UIHandler) MarkNotificationReadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, _ := authctx.UserID(ctx)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if h.notifSvc != nil && id > 0 {
		_ = h.notifSvc.MarkRead(ctx, id, userID)
	}

	w.Header().Set("HX-Trigger", "notificationRead")

	if r.URL.Query().Get("format") == "dropdown" || r.Header.Get("HX-Target") == "notif-dropdown-content" {
		var logs []*notifications.NotificationLog
		unread := 0
		if h.notifSvc != nil {
			logs, _ = h.notifSvc.ListUserNotifications(ctx, userID, 8, 0)
			unread, _ = h.notifSvc.GetUnreadCount(ctx, userID)
		}
		if actor, ok := authctx.From(ctx); ok {
			logs = filterNotificationsForActor(actor, logs)
		}
		h.renderPage(ctx, w, "render notifications dropdown", pages.NotificationsDropdownPanel(logs, unread))
		return
	}

	http.Redirect(w, r, "/notifications", http.StatusSeeOther)
}
