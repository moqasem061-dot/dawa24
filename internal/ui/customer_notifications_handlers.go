package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/notifications"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
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

	unread, loadErr := h.notifSvc.GetUnreadCount(ctx, userID)
	h.logLoadErr(ctx, "notifSvc.GetUnreadCount", loadErr)

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
			if loaded, err := h.notifSvc.ListUserNotifications(ctx, userID, 8, 0); err != nil {
				h.logLoadErr(ctx, "notifSvc.ListUserNotifications", err)
			} else {
				logs = loaded
			}
			if loaded, err := h.notifSvc.GetUnreadCount(ctx, userID); err != nil {
				h.logLoadErr(ctx, "notifSvc.GetUnreadCount", err)
			} else {
				unread = loaded
			}
		}
		if actor, ok := authctx.From(ctx); ok {
			logs = filterNotificationsForActor(actor, logs)
		}
		h.renderPage(ctx, w, "render notifications dropdown", pages.NotificationsDropdownPanel(logs, unread))
		return
	}

	http.Redirect(w, r, "/notifications", http.StatusSeeOther)
}

// NotificationOpenRedirect marks a notification as read and redirects the user to its relevant target.
func (h *UIHandler) NotificationOpenRedirect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	targetURL := fallbackDashboard(actor)

	if h.notifSvc != nil && id > 0 {
		var notif *notifications.NotificationLog
		if logs, err := h.notifSvc.ListUserNotifications(ctx, actor.UserID, 100, 0); err != nil {
			h.logLoadErr(ctx, "notifSvc.ListUserNotifications", err)
		} else {
			for _, l := range logs {
				if l != nil && l.ID == id {
					notif = l
					break
				}
			}
		}
		if notif != nil {
			targetURL = h.ResolveNotificationDestination(ctx, actor, notif)
		}
		_ = h.notifSvc.MarkRead(ctx, id, actor.UserID)
	}

	w.Header().Set("HX-Trigger", "notificationRead")

	if h.isHTMX(r) {
		w.Header().Set("HX-Redirect", targetURL)
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, targetURL, http.StatusSeeOther)
}
