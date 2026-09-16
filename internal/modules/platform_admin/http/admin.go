package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// RegisterAdminRoutes mounts administrative platform routes.
func (h *Handler) RegisterAdminRoutes(r chi.Router) {
	r.Group(func(admin chi.Router) {
		admin.Use(authctx.RequirePermission("platform.admin"))

		admin.Get("/api/v1/admin/platform/settings", h.ListPublicSettings)
		admin.Put("/api/v1/admin/platform/settings/{key}", h.SetSetting)
		admin.Get("/api/v1/admin/platform/translations", h.AdminListTranslations)
		admin.Put("/api/v1/admin/platform/translations/{key}", h.AdminUpdateTranslation)
		admin.Get("/api/v1/admin/platform/audit-log", h.AdminViewAuditLog)
	})
}

func (h *Handler) AdminListTranslations(w http.ResponseWriter, r *http.Request) {
	// TODO: list translations
	httpx.JSON(w, http.StatusOK, map[string]any{"translations": []any{}})
}

func (h *Handler) AdminUpdateTranslation(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		httpx.Error(w, r, h.log, apperr.Validation("key.invalid", "Invalid key", nil))
		return
	}
	// TODO: update translation
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "updated", "key": key})
}

func (h *Handler) AdminViewAuditLog(w http.ResponseWriter, r *http.Request) {
	// TODO: view audit log
	httpx.JSON(w, http.StatusOK, map[string]any{"audit_log": []any{}})
}
