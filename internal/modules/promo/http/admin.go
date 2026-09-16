package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
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

func (h *Handler) AdminListPackages(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	packages, err := h.service.AdminListPackages(ctx)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"packages": packages})
}

func (h *Handler) AdminCreatePackage(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	var p promo.OfferPackage
	if err := httpx.DecodeJSON(w, r, &p); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	created, err := h.service.AdminCreatePackage(ctx, &p)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}
