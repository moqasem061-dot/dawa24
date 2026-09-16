package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// RegisterAdminRoutes mounts administrative identity routes.
func (h *Handler) RegisterAdminRoutes(r chi.Router) {
	r.Group(func(admin chi.Router) {
		admin.Use(httpx.CSRF(h.secureCookie))
		admin.Use(RequireAuth(h.service, h.resolver, h.cookieName, h.log))
		admin.Use(authctx.RequirePermission("identity.admin"))

		admin.Get("/api/v1/admin/identity/users", h.AdminListUsers)
		admin.Get("/api/v1/admin/identity/users/{id}", h.AdminGetUser)
		admin.Post("/api/v1/admin/identity/users/{id}/suspend", h.AdminSuspendUser)
		admin.Post("/api/v1/admin/identity/users/{id}/reactivate", h.AdminReactivateUser)
		admin.Post("/api/v1/admin/identity/users/{id}/reset-mfa", h.AdminResetMFA)
		admin.Put("/api/v1/admin/identity/users/{id}/role", h.AdminAssignRole)
	})
}

func (h *Handler) AdminListUsers(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	role := r.URL.Query().Get("role")
	status := r.URL.Query().Get("status")

	users, err := h.service.AdminListUsers(ctx, role, status)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"users": users})
}

func (h *Handler) AdminGetUser(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid user ID", nil))
		return
	}

	user, err := h.service.AdminGetUser(ctx, id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, user)
}

func (h *Handler) AdminSuspendUser(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	actorID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid user ID", nil))
		return
	}

	if err := h.service.AdminSuspendUser(ctx, id, actorID); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "suspended"})
}

func (h *Handler) AdminReactivateUser(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	actorID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid user ID", nil))
		return
	}

	if err := h.service.AdminReactivateUser(ctx, id, actorID); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "active"})
}

func (h *Handler) AdminResetMFA(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	actorID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid user ID", nil))
		return
	}

	if err := h.service.AdminResetMFA(ctx, id, actorID); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "mfa_reset"})
}

func (h *Handler) AdminAssignRole(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	actorID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid user ID", nil))
		return
	}

	var body struct {
		Role string `json:"role"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.AdminAssignRole(ctx, id, body.Role, actorID); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "role_assigned"})
}
