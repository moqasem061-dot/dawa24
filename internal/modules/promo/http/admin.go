package http

import (
	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

// RegisterAdminRoutes mounts administrative promo routes.
func (h *Handler) RegisterAdminRoutes(r chi.Router) {
	h.RegisterAdminSponsorshipRoutes(r)

	r.Group(func(admin chi.Router) {
		admin.Use(authctx.RequirePermission("promo.admin"))

		admin.Get("/api/v1/admin/promo/ads", h.AdminListAllAds)
		admin.Post("/api/v1/admin/promo/ads/{id}/approve", h.AdminApproveAd)
		admin.Post("/api/v1/admin/promo/ads/{id}/reject", h.AdminRejectAd)
		admin.Get("/api/v1/admin/promo/sponsorships", h.AdminListSponsorshipRequests)
		admin.Post("/api/v1/admin/promo/sponsorships/{id}/review", h.AdminApproveSponsorshipRequest)
		admin.Get("/api/v1/admin/promo/packages", h.AdminListSponsorshipPackages)
		admin.Post("/api/v1/admin/promo/packages", h.AdminCreateSponsorshipPackage)
	})
}
