package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// RegisterAdminSponsorshipRoutes mounts admin sponsorship and ad moderation endpoints.
func (h *Handler) RegisterAdminSponsorshipRoutes(r chi.Router) {
	r.Group(func(admin chi.Router) {
		admin.Use(authctx.RequirePermission("promo.admin"))

		// Sponsorship packages
		admin.Get("/api/v1/admin/promo/packages", h.AdminListSponsorshipPackages)
		admin.Post("/api/v1/admin/promo/packages", h.AdminCreateSponsorshipPackage)
		admin.Put("/api/v1/admin/promo/packages/{id}", h.AdminUpdateSponsorshipPackage)
		admin.Post("/api/v1/admin/promo/packages/{id}/toggle", h.AdminToggleSponsorshipPackage)

		// Sponsorship requests moderation
		admin.Get("/api/v1/admin/promo/sponsorship-requests", h.AdminListSponsorshipRequests)
		admin.Post("/api/v1/admin/promo/sponsorship-requests/{id}/approve", h.AdminApproveSponsorshipRequest)
		admin.Post("/api/v1/admin/promo/sponsorship-requests/{id}/reject", h.AdminRejectSponsorshipRequest)

		// Ad moderation
		admin.Get("/api/v1/admin/promo/ads", h.AdminListAllAds)
		admin.Get("/api/v1/admin/promo/ads/pending", h.AdminListPendingAds)
		admin.Post("/api/v1/admin/promo/ads/{id}/approve", h.AdminApproveAd)
		admin.Post("/api/v1/admin/promo/ads/{id}/reject", h.AdminRejectAd)
	})
}

// AdminListSponsorshipPackages returns all packages for admin management.
func (h *Handler) AdminListSponsorshipPackages(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	packages, err := h.service.AdminListPackages(ctx)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"packages": packages})
}

// AdminCreateSponsorshipPackage creates a sponsorship package.
func (h *Handler) AdminCreateSponsorshipPackage(w http.ResponseWriter, r *http.Request) {
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

// AdminUpdateSponsorshipPackage updates a sponsorship package.
func (h *Handler) AdminUpdateSponsorshipPackage(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid package ID", nil))
		return
	}
	var p promo.OfferPackage
	if err := httpx.DecodeJSON(w, r, &p); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	p.ID = id
	updated, err := h.service.AdminUpdatePackage(ctx, &p)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

// AdminToggleSponsorshipPackage activates or deactivates a package.
func (h *Handler) AdminToggleSponsorshipPackage(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid package ID", nil))
		return
	}
	active := r.PostFormValue("active") == "true" || r.PostFormValue("active") == "on"
	if err := h.service.AdminTogglePackageActive(ctx, id, active); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"id": id, "active": active})
}

// AdminListSponsorshipRequests returns all sponsorship requests for moderation.
func (h *Handler) AdminListSponsorshipRequests(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 100
	}
	requests, err := h.service.AdminListSponsorshipRequests(ctx, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"requests": requests, "count": len(requests)})
}

// AdminApproveSponsorshipRequest approves a pending sponsorship request.
func (h *Handler) AdminApproveSponsorshipRequest(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid request ID", nil))
		return
	}
	notes := r.PostFormValue("notes")
	sr, err := h.service.AdminApproveSponsorshipRequest(ctx, id, notes)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, sr)
}

// AdminRejectSponsorshipRequest rejects a pending sponsorship request.
func (h *Handler) AdminRejectSponsorshipRequest(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid request ID", nil))
		return
	}
	notes := r.PostFormValue("notes")
	if err := h.service.AdminRejectSponsorshipRequest(ctx, id, notes); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"id": id, "status": "rejected"})
}

// AdminListAllAds returns all ads for admin moderation.
func (h *Handler) AdminListAllAds(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 100
	}
	ads, err := h.service.AdminListAds(ctx, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ads": ads, "count": len(ads)})
}

// AdminListPendingAds returns only pending ads awaiting review.
func (h *Handler) AdminListPendingAds(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	ads, err := h.service.AdminListAds(ctx, 200, 0)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	var pending []*promo.Ad
	for _, a := range ads {
		if a != nil && a.AdminStatus == promo.AdminPending {
			pending = append(pending, a)
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ads": pending, "count": len(pending)})
}

// AdminApproveAd approves an ad for display.
func (h *Handler) AdminApproveAd(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid ad ID", nil))
		return
	}
	notes := r.PostFormValue("notes")
	if err := h.service.AdminApproveAd(ctx, id, notes); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"id": id, "status": "approved"})
}

// AdminRejectAd rejects an ad.
func (h *Handler) AdminRejectAd(w http.ResponseWriter, r *http.Request) {
	ctx := database.AsSystem(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid ad ID", nil))
		return
	}
	notes := r.PostFormValue("notes")
	if err := h.service.AdminRejectAd(ctx, id, notes); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"id": id, "status": "rejected"})
}
