package ui

import (
	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

// registerAdminMatchingRoutes mounts diagnostics and troubleshooting endpoints for product matching.
func (h *UIHandler) registerAdminMatchingRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.matching.diagnose"))
		g.Get("/admin/matching/runs/{id}/diagnose", h.AdminMatchDiagnosticsPage)
	})
}
