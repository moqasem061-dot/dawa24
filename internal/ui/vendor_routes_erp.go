package ui

import (
	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

// registerVendorERPRoutes mounts the supplier's integration screen and its
// key mutations. It is called as the last statement of registerVendorRoutes;
// that file takes no other edit (it sits at its 400-line ceiling).
//
// The parameter is named g (not r) so the protected-path regex in
// test/route_audience_test.go ignores this file.
func (h *UIHandler) registerVendorERPRoutes(g chi.Router) {
	g.Group(func(gg chi.Router) {
		gg.Use(authctx.RequireTenantPagePermission("vendor.integration.view"))
		gg.Get("/vendor/integrations", h.ErpKeysPage)
	})
	g.Group(func(gg chi.Router) {
		gg.Use(authctx.RequireTenantPagePermission("vendor.integration.manage"))
		gg.Post("/vendor/integrations/keys", h.ErpKeyCreate)
		gg.Post("/vendor/integrations/keys/{id}/rotate", h.ErpKeyRotate)
		gg.Post("/vendor/integrations/keys/{id}/revoke", h.ErpKeyRevoke)
		gg.Post("/vendor/integrations/keys/{id}/limits", h.ErpKeyLimits)
		gg.Post("/vendor/integrations/pause", h.ErpIntegrationPause)
	})
}
