package http

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Handler exposes promotional and advertising endpoints.
type Handler struct {
	service          *promo.Service
	trustedProxyHops int
	log              *slog.Logger
}

// SetTrustedProxyHops configures the hop count for trusted reverse proxies.
func (h *Handler) SetTrustedProxyHops(n int) { h.trustedProxyHops = n }

// NewHandler creates a promo HTTP handler.
func NewHandler(service *promo.Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes registers promo routes on a Chi router.
//
// Click recording stays open to any authenticated caller: it is a counter on
// somebody else's advertisement and refusing it would break the metric, not
// protect anything. Creating an offer or a homepage highlight is not — a
// highlight section is the platform's own storefront, and an offer is a
// commercial commitment by a supplier.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/promo/offers", h.ListOffers)
	r.Get("/api/v1/promo/offers/{id}", h.GetOffer)
	r.Post("/api/v1/promo/offers/{id}/click", h.RecordClick)

	r.Get("/api/v1/promo/packages", h.ListPackages)
	r.Get("/api/v1/promo/ads", h.ListAds)
	r.Post("/api/v1/promo/ads/{id}/click", h.RecordAdClick)

	r.Get("/api/v1/promo/highlights", h.ListHighlights)

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePermission("vendor.offer.manage", "promo.offer.manage", "promo.admin"))
		g.Post("/api/v1/promo/offers", h.CreateOffer)
	})
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireAPIPermission("platform.content.update", "promo.offer.manage", "promo.admin"))
		g.Post("/api/v1/promo/highlights", h.CreateHighlight)
	})

	h.RegisterAdminRoutes(r)
	h.RegisterVendorSponsorshipRoutes(r)
}

// ListOffers returns all currently active promotions.
func (h *Handler) ListOffers(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	offers, err := h.service.ListActiveOffers(r.Context(), limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"offers": offers,
		"count":  len(offers),
	})
}

// CreateOffer handles offer creation.
func (h *Handler) CreateOffer(w http.ResponseWriter, r *http.Request) {
	var o promo.Offer
	if err := httpx.DecodeJSON(w, r, &o); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	created, err := h.service.CreateOffer(r.Context(), &o)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, created)
}

// GetOffer retrieves an offer by ID and tracks a view impression.
func (h *Handler) GetOffer(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid offer ID", nil))
		return
	}

	offer, err := h.service.GetOffer(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	_ = h.service.RecordOfferView(r.Context(), id)
	httpx.JSON(w, http.StatusOK, offer)
}

// RecordClick increments click analytics for an offer.
func (h *Handler) RecordClick(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid offer ID", nil))
		return
	}

	_ = h.service.RecordOfferClick(r.Context(), id)
	w.WriteHeader(http.StatusNoContent)
}

// ListPackages returns sponsorship packages.
func (h *Handler) ListPackages(w http.ResponseWriter, r *http.Request) {
	packages, err := h.service.ListPackages(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"packages": packages,
	})
}

// ListAds returns banner ads for a given position.
func (h *Handler) ListAds(w http.ResponseWriter, r *http.Request) {
	position := r.URL.Query().Get("position")
	ads, err := h.service.ListActiveAds(r.Context(), position)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"ads": ads,
	})
}

// RecordAdClick tracks ad click events.
func (h *Handler) RecordAdClick(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid ad ID", nil))
		return
	}

	_ = h.service.RecordAdClick(r.Context(), id, nil, httpx.ClientIP(r, h.trustedProxyHops), r.UserAgent())
	w.WriteHeader(http.StatusNoContent)
}

// ListHighlights returns homepage highlight sections.
func (h *Handler) ListHighlights(w http.ResponseWriter, r *http.Request) {
	sections, err := h.service.ListHighlightSections(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"highlights": sections, "count": len(sections)})
}

// CreateHighlight creates a highlight section.
func (h *Handler) CreateHighlight(w http.ResponseWriter, r *http.Request) {
	var sec promo.HighlightSection
	if err := httpx.DecodeJSON(w, r, &sec); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	created, err := h.service.CreateHighlightSection(r.Context(), &sec)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}
