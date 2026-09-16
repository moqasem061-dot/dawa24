package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

// RegisterAdminRoutes mounts administrative workflow routes.
func (h *Handler) RegisterAdminRoutes(r chi.Router) {
	r.Group(func(admin chi.Router) {
		admin.Use(authctx.RequirePermission("workflow.admin"))

		admin.Get("/api/v1/admin/workflow/issues", h.AdminListIssues)
	})
}

func (h *Handler) AdminListIssues(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	issues, err := h.service.ListIssues(ctx, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"issues": issues})
}
