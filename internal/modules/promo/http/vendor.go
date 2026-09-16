package http

import (
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// RegisterVendorSponsorshipRoutes mounts the vendor sponsorship endpoints.
//
// Purchasing a sponsorship package spends the organisation's wallet balance and
// submitting a sponsorship request commits it to a placement. Both were open to
// any member of an approved organisation, while the screens that do the same
// things require vendor.offer_package.manage and vendor.ad.manage.
//
// Impression recording stays open: it is a counter, and a gate on it would lose
// the metric rather than protect anything.
func (h *Handler) RegisterVendorSponsorshipRoutes(r chi.Router) {
	r.Get("/api/v1/vendor/sponsorship/packages", h.VendorListSponsorshipPackages)
	r.Get("/api/v1/vendor/sponsorship/purchases", h.VendorListPurchases)
	r.Get("/api/v1/vendor/sponsorship/active-purchases", h.VendorListActivePurchases)
	r.Get("/api/v1/vendor/sponsorship/requests", h.VendorListSponsorshipRequests)
	r.Get("/api/v1/vendor/ads", h.VendorListAds)
	r.Post("/api/v1/vendor/ads/{id}/impression", h.VendorRecordAdImpression)

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("vendor.offer_package.manage", "promo.offer.manage", "promo.admin"))
		g.Post("/api/v1/vendor/sponsorship/packages/{id}/purchase", h.VendorPurchasePackage)
		g.Post("/api/v1/vendor/sponsorship/requests", h.VendorSubmitSponsorshipRequest)
		g.Delete("/api/v1/vendor/sponsorship/requests/{id}", h.VendorCancelSponsorshipRequest)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("vendor.ad.manage", "promo.ad.update", "promo.admin"))
		g.Post("/api/v1/vendor/ads", h.VendorCreateAd)
		g.Put("/api/v1/vendor/ads/{id}", h.VendorUpdateAd)
	})
}

// VendorListSponsorshipPackages returns available sponsorship packages.
func (h *Handler) VendorListSponsorshipPackages(w http.ResponseWriter, r *http.Request) {
	packages, err := h.service.ListPackages(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"packages": packages})
}

// VendorPurchasePackage purchases a sponsorship package using wallet credits.
func (h *Handler) VendorPurchasePackage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid package ID", nil))
		return
	}
	autoRenew := r.PostFormValue("auto_renew") == "true" || r.PostFormValue("auto_renew") == "on"
	billingCycle := r.PostFormValue("billing_cycle")

	purchase, err := h.service.PurchaseSponsorshipPackage(r.Context(), id, autoRenew, billingCycle)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, purchase)
}

// VendorListPurchases returns all the vendor's sponsorship purchases.
func (h *Handler) VendorListPurchases(w http.ResponseWriter, r *http.Request) {
	purchases, err := h.service.ListSponsorshipPurchases(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"purchases": purchases})
}

// VendorListActivePurchases returns only active purchases with remaining credits.
func (h *Handler) VendorListActivePurchases(w http.ResponseWriter, r *http.Request) {
	purchases, err := h.service.ListActiveSponsorshipPurchases(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"purchases": purchases, "count": len(purchases)})
}

// VendorSubmitSponsorshipRequest creates a sponsorship request for a product or offer.
func (h *Handler) VendorSubmitSponsorshipRequest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ItemType  string `json:"item_type"`
		ItemID    int64  `json:"item_id"`
		PackageID int64  `json:"package_id"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	if req.ItemType != "product" && req.ItemType != "offer" {
		httpx.Error(w, r, h.log, apperr.Validation("item_type.invalid", i18n.TDefault("w4_mod.w4str_251_251"), nil))
		return
	}
	if req.ItemID <= 0 || req.PackageID <= 0 {
		httpx.Error(w, r, h.log, apperr.Validation("ids.required", i18n.TDefault("w4_mod.w4str_252_252"), nil))
		return
	}

	sr, err := h.service.SubmitSponsorshipRequest(r.Context(), promo.SponsorshipItemType(req.ItemType), req.ItemID, req.PackageID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, sr)
}

// VendorListSponsorshipRequests returns the vendor's sponsorship requests.
func (h *Handler) VendorListSponsorshipRequests(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 50
	}
	requests, err := h.service.ListSponsorshipRequestsByOrg(r.Context(), limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"requests": requests, "count": len(requests)})
}

// VendorCancelSponsorshipRequest cancels a pending sponsorship request.
func (h *Handler) VendorCancelSponsorshipRequest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid request ID", nil))
		return
	}
	if err := h.service.CancelSponsorshipRequest(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// VendorListAds returns the vendor's advertisements.
func (h *Handler) VendorListAds(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 50
	}
	ads, err := h.service.ListAdsByOrg(r.Context(), limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"ads": ads, "count": len(ads)})
}

// VendorCreateAd creates a new advertisement.
func (h *Handler) VendorCreateAd(w http.ResponseWriter, r *http.Request) {
	var a promo.Ad
	if err := httpx.DecodeJSON(w, r, &a); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	created, err := h.service.CreateAd(r.Context(), &a)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}

// VendorUpdateAd updates an existing advertisement.
func (h *Handler) VendorUpdateAd(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid ad ID", nil))
		return
	}
	var a promo.Ad
	if err := httpx.DecodeJSON(w, r, &a); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	a.ID = id
	if err := h.service.UpdateAd(r.Context(), &a); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, a)
}

// VendorRecordAdImpression logs an ad impression (view).
func (h *Handler) VendorRecordAdImpression(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid ad ID", nil))
		return
	}
	var userID *int64
	if uid, uErr := authctx.UserID(r.Context()); uErr == nil && uid > 0 {
		userID = &uid
	}
	_ = h.service.RecordAdImpression(r.Context(), id, userID, httpx.ClientIP(r, h.trustedProxyHops), r.UserAgent())
	w.WriteHeader(http.StatusNoContent)
}
