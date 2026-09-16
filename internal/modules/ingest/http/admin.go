package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

// RegisterAdminRoutes mounts administrative ingest routes.
func (h *Handler) RegisterAdminRoutes(r chi.Router) {
	r.Group(func(admin chi.Router) {
		// These handlers read and write across every tenant with
		// database.AsSystem. Without this guard the whole group was reachable
		// by any authenticated user, matching neither the other modules nor
		// the intent of an /admin/ path.
		admin.Use(authctx.RequirePermission("ingest.admin"))

		admin.Get("/api/v1/admin/ingest/sessions", h.AdminListSessions)
	})
}

func (h *Handler) AdminListSessions(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	sessions, err := h.service.ListSessions(ctx, 0, 50, 0)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"sessions": sessions, "count": len(sessions)})
}
