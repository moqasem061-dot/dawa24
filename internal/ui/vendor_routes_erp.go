package ui

import (
	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/erpgate"
)

// registerVendorERPRoutes mounts the supplier's integration screen and its
// key mutations. It is called as the last statement of registerVendorRoutes;
// that file takes no other edit (it sits at its 400-line ceiling).
//
// The parameter is named g (not r) so the protected-path regex in
// test/route_audience_test.go ignores this file.
func (h *UIHandler) registerVendorERPRoutes(g chi.Router) {
	if h.erpSvc == nil {
		// The ERP API is disabled on this deployment (SetERP(nil)); the
		// navigation entry is hidden by rbac.NavSuppressions and these routes
		// do not exist.
		return
	}
	// The runtime switch: while an admin has the integration off, every route
	// below answers 404 as though it did not exist.
	g = g.With(erpgate.RequireEnabled)
	g.Group(func(gg chi.Router) {
		gg.Use(authctx.RequireTenantPagePermission("vendor.integration.view"))
		gg.Get("/vendor/integrations", h.ErpKeysPage)
		// The activity screen: calls, product syncs, acknowledged orders, the
		// live panel fragment and one sync's items (plan section 3c).
		gg.Get("/vendor/integrations/activity", h.ErpActivityPage)
		gg.Get("/vendor/integrations/activity/live", h.ErpActivityLive)
		gg.Get("/vendor/integrations/activity/syncs/{id}", h.ErpBatchPage)
	})
	g.Group(func(gg chi.Router) {
		gg.Use(authctx.RequireTenantPagePermission("vendor.integration.review"))
		gg.Get("/vendor/integrations/review", h.ErpReviewPage)
		// The product search of the review form, under the review permission so
		// a reviewer does not also need the product-catalogue page.
		gg.Get("/vendor/integrations/review/search-json", h.VendorCatalogSearchJSON)
		gg.Post("/vendor/integrations/review/{id}/resolve", h.ErpReviewResolve)
		gg.Post("/vendor/integrations/review/{id}/dismiss", h.ErpReviewDismiss)
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
