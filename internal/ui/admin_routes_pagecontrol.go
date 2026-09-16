package ui

import (
	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// The System Pages surface, /admin/system-pages.
//
// Reading the catalogue, adding a page, toggling one and deleting one are four
// separate rights: whoever can disable a page can hide any part of the platform,
// so the write keys are withheld from the starter "Administrator" role and held
// by super_admin alone (internal/platform/rbac/roles.go).
func (h *UIHandler) registerAdminPageControlRoutes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.page_control.view"))
		g.Get("/admin/system-pages", h.AdminSystemPagesPage)
		g.Get("/admin/system-pages/preview-maintenance", h.AdminSystemPagesMaintenancePreview)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.page_control.update"))
		g.Post("/admin/system-pages/{id}/toggle", h.AdminSystemPageToggleSubmit)
		g.Post("/admin/system-pages/{id}/edit", h.AdminSystemPageEditSubmit)
		g.Post("/admin/system-pages/rescan", h.AdminSystemPageRescanSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.page_control.create"))
		g.Post("/admin/system-pages", h.AdminSystemPageCreateSubmit)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("platform.page_control.delete"))
		g.Post("/admin/system-pages/{id}/delete", h.AdminSystemPageDeleteSubmit)
	})
}
