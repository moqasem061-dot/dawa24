package ui

import (
	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

// registerAdminMatchingRoutes mounts diagnostics and troubleshooting endpoints for product matching.
func (h *UIHandler) registerAdminMatchingRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.match_decision.view"))
		g.Get("/admin/matching/runs/{id}/diagnose", h.AdminMatchDiagnosticsPage)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.developer.sql"))
		g.Get("/admin/matching/ai-configs", h.AdminAIMatchConfigsPage)
		g.Post("/admin/matching/ai-configs/{id}/status", h.AdminAIMatchConfigUpdateStatus)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("catalog.match_decision.view"))
		g.Get("/admin/matching/policies", h.AdminMatchPoliciesPage)
		g.Post("/admin/matching/policies/assign", h.AdminMatchPolicyUpdateAssignment)
		g.Post("/admin/matching/policies/rollback", h.AdminMatchPolicyRollback)
	})
}

