package ui

import (
	"net/http"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// NotificationsDropdownPartial renders the bell dropdown panel as an HTMX partial.
func (h *UIHandler) NotificationsDropdownPartial(w http.ResponseWriter, r *http.Request) {
	if !h.isHTMX(r) {
		http.Redirect(w, r, "/notifications", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		h.renderPage(ctx, w, "render notifications dropdown fallback", pages.NotificationsDropdownPanel(nil, 0))
		return
	}

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
}

// NotificationsUnreadBadgePartial renders the polled unread badge.
func (h *UIHandler) NotificationsUnreadBadgePartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		h.renderPage(ctx, w, "render unread badge fallback", pages.NotificationsUnreadBadge(0))
		return
	}

	unread := 0
	if h.notifSvc != nil {
		unread, _ = h.notifSvc.GetUnreadCount(ctx, userID)
	}

	h.renderPage(ctx, w, "render unread badge", pages.NotificationsUnreadBadge(unread))
}

// NotificationsReadAllSubmit marks every notification as read and returns the
// refreshed panel so the badge clears without a full page reload when requested via HTMX dropdown,
// or redirects safely to the notifications page for standard form submissions.
func (h *UIHandler) NotificationsReadAllSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		if h.isHTMX(r) && r.Header.Get("HX-Target") == "notif-dropdown-content" {
			h.renderPage(ctx, w, "render notifications dropdown fallback", pages.NotificationsDropdownPanel(nil, 0))
			return
		}
		http.Redirect(w, r, "/auth/login?redirect=/notifications", http.StatusSeeOther)
		return
	}

	if h.notifSvc != nil {
		_, _ = h.notifSvc.MarkAllRead(ctx, userID)
	}

	isDropdownHTMX := h.isHTMX(r) && (r.Header.Get("HX-Target") == "notif-dropdown-content" || r.URL.Query().Get("format") == "dropdown")
	if !isDropdownHTMX {
		ref := r.Header.Get("Referer")
		if ref != "" && !strings.Contains(ref, "/notifications/read-all") {
			http.Redirect(w, r, ref, http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/notifications", http.StatusSeeOther)
		return
	}

	var logs []*notifications.NotificationLog
	if h.notifSvc != nil {
		logs, _ = h.notifSvc.ListUserNotifications(ctx, userID, 8, 0)
	}
	if actor, ok := authctx.From(ctx); ok {
		logs = filterNotificationsForActor(actor, logs)
	}

	h.renderPage(ctx, w, "render notifications dropdown panel", pages.NotificationsDropdownPanel(logs, 0))
}

// filterNotificationsForActor filters out notification logs that the authenticated actor does not have permission to see.
// This provides an in-memory defense-in-depth layer on top of PostgreSQL permission filtering.
func filterNotificationsForActor(actor authctx.Actor, logs []*notifications.NotificationLog) []*notifications.NotificationLog {
	if len(logs) == 0 {
		return logs
	}
	// Only super administrators with global wildcard authority bypass notification permission filtering.
	if actor.IsSuperAdmin() || actor.Can("*") {
		return logs
	}

	filtered := make([]*notifications.NotificationLog, 0, len(logs))
	for _, l := range logs {
		if l == nil {
			continue
		}

		// Branch scoping: If user is bound to a specific branch, exclude notifications from other branches.
		if actor.BoundBranchID != nil && *actor.BoundBranchID > 0 && l.BranchID != nil && *l.BranchID > 0 && *l.BranchID != *actor.BoundBranchID {
			continue
		}

		// Organization owners see all non-branch-mismatched notifications for their tenant.
		if actor.IsOwner && actor.DashboardScope() != rbac.ScopeAdmin {
			filtered = append(filtered, l)
			continue
		}

		perm := l.RequiredPermission
		if perm == "" {
			perm = inferNotificationPermission(l.Title, l.Body)
		}

		// If a permission is required and actor doesn't hold it, exclude
		if perm != "" && !actor.Can(perm) {
			continue
		}

		// Hard exclusion for couriers / delivery personnel: never show vendor orders, purchase requests, or wallet transactions
		isCourier := actor.Role == "courier" || actor.Role == "org_courier" || (!actor.Can("vendor.order.view") && actor.Can("vendor.delivery.view"))
		if isCourier {
			if perm == "vendor.order.view" || perm == "vendor.wallet.view" || perm == "vendor.purchase_request.view" ||
				strings.Contains(l.Title, "طلب توريد") || strings.Contains(l.Title, "شحن رصيد") || strings.Contains(l.Title, "إيداع") ||
				strings.Contains(l.Title, "سحب") || strings.Contains(l.Title, "محفظة") {
				continue
			}
		}

		filtered = append(filtered, l)
	}
	return filtered
}

// inferNotificationPermission deduces the required RBAC permission from notification title and body.
func inferNotificationPermission(title, body string) string {
	combined := title + " " + body

	// Platform admin approvals & documents
	if strings.Contains(combined, "اعتماد منشأة") || strings.Contains(combined, "تسجيل منشأة جديدة") || strings.Contains(combined, "طلب اعتماد") {
		return "org.approval.view"
	}
	if strings.Contains(combined, "ترخيص") || strings.Contains(combined, "مستند جديد") || strings.Contains(combined, "مستندات المنشأة") {
		return "hr.document.view"
	}

	// Supply orders (vendor side)
	if strings.Contains(combined, "طلب توريد") || strings.Contains(combined, "توريد جديد") {
		return "vendor.order.view"
	}
	// Customer / Pharmacy orders
	if strings.Contains(combined, "تم تأكيد طلبك") || strings.Contains(combined, "تم استلام طلبك") ||
		strings.Contains(combined, "تم شحن طلبك") || strings.Contains(combined, "تم تسليم طلبك") ||
		strings.Contains(combined, "إلغاء الطلب") || strings.Contains(combined, "تعديل طلب") {
		return "pharmacy.order.view"
	}
	// Wallet / Finance
	if strings.Contains(combined, "شحن رصيد") || strings.Contains(combined, "إيداع") ||
		strings.Contains(combined, "سحب رصيد") || strings.Contains(combined, "سحب أرباح") ||
		strings.Contains(combined, "محفظتك") || strings.Contains(combined, "المحفظة") {
		return "vendor.wallet.view"
	}
	// Purchase Requests / RFQ
	if strings.Contains(combined, "طلب شراء جديد") || strings.Contains(combined, "طلب تسعير") ||
		strings.Contains(combined, "عرض أسعار جديد") {
		return "vendor.purchase_request.view"
	}
	// Delivery parcels
	if strings.Contains(combined, "طرد جديد") || strings.Contains(combined, "تفويض مندوب") ||
		strings.Contains(combined, "تم تعيينك") || strings.Contains(combined, "استلام الطرد") {
		return "vendor.delivery.view"
	}
	// Offers / Ads
	if strings.Contains(combined, "عرض خاص") {
		return "vendor.offer.view"
	}
	if strings.Contains(combined, "باقة رعاية") {
		return "vendor.offer_package.view"
	}
	if strings.Contains(combined, "إعلانك") {
		return "vendor.ad.view"
	}

	return ""
}
