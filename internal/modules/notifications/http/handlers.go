package http

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
)

// Handler exposes notification endpoints.
type Handler struct {
	service *notifications.Service
	log     *slog.Logger
}

// NewHandler creates a notification HTTP handler.
func NewHandler(service *notifications.Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes registers notification routes on a Chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/notifications", h.ListNotifications)
	r.Post("/api/v1/notifications/{id}/read", h.MarkAsRead)
	r.Get("/api/v1/notifications/unread-count", h.GetUnreadCount)
	r.Get("/api/v1/notifications/unread", h.ListUnread)
	r.Post("/api/v1/notifications/read-all", h.MarkAllRead)

	h.RegisterAdminRoutes(r)
}

// ListNotifications returns in-app notification feed.
func (h *Handler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	// The acting user comes from the authenticated session, never from the
	// request. Reading it from the query string let any caller act as any
	// user by changing a number.
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	list, err := h.service.ListUserNotifications(r.Context(), userID, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"notifications": list,
		"count":         len(list),
	})
}

// MarkAsRead marks a message as seen.
func (h *Handler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid notification ID", nil))
		return
	}

	// The acting user comes from the authenticated session, never from the
	// request. Reading it from the query string let any caller act as any
	// user by changing a number.
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.MarkRead(r.Context(), id, userID); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{"status": "marked_read"})
}

// GetUnreadCount returns total unread messages count.
func (h *Handler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	// The acting user comes from the authenticated session, never from the
	// request. Reading it from the query string let any caller act as any
	// user by changing a number.
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	count, err := h.service.GetUnreadCount(r.Context(), userID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"unread_count": count})
}

// ListUnread returns only unread notifications, for the badge dropdown.
func (h *Handler) ListUnread(w http.ResponseWriter, r *http.Request) {
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	p := pagination.FromRequest(r)
	list, err := h.service.ListUnread(r.Context(), userID, p.Limit, p.Offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, pagination.Page[*notifications.NotificationLog]{
		Data:    list,
		HasMore: len(list) == p.Limit,
	})
}

// MarkAllRead clears the caller's whole notification feed.
func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	updated, err := h.service.MarkAllRead(r.Context(), userID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"marked_read": updated})
}
