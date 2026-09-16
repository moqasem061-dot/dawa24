package http

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Handler exposes platform administration endpoints.
type Handler struct {
	service *platformadmin.Service
	log     *slog.Logger
}

// NewHandler creates a platform admin HTTP handler.
func NewHandler(service *platformadmin.Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes registers platform admin routes on a Chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/platform/settings/public", h.ListPublicSettings)
	r.Get("/api/v1/platform/settings/{key}", h.GetSetting)

	// Writing a platform setting was reachable by any authenticated member of
	// any approved organisation. platform_admin.system_settings holds the
	// platform's own configuration — branding, contact details, feature
	// switches — and the admin screen that edits it has always required
	// platform.setting.update. This is the same gate on the same table.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequireAPIPermission("platform.setting.update", "platform.admin"))
		g.Put("/api/v1/platform/settings/{key}", h.SetSetting)
	})
	r.Get("/api/v1/platform/countries", h.ListCountries)
	r.Get("/api/v1/platform/countries/{id}/cities", h.ListCities)

	r.Get("/api/v1/platform/currencies", h.ListCurrencies)
	r.Get("/api/v1/platform/languages", h.ListLanguages)
	r.Post("/api/v1/platform/contact", h.SubmitContact)
	r.Get("/api/v1/platform/contact", h.ListContactMessages)

	h.RegisterAdminRoutes(r)
}

// ListPublicSettings returns public configs.
func (h *Handler) ListPublicSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.service.ListPublicSettings(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"settings": settings})
}

// GetSetting retrieves a setting.
func (h *Handler) GetSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	s, err := h.service.GetSetting(r.Context(), key)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, s)
}

// SetSetting writes a setting.
func (h *Handler) SetSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var s platformadmin.SystemSetting
	if err := httpx.DecodeJSON(w, r, &s); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	s.Key = key

	if err := h.service.SetSetting(r.Context(), &s); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// ListCountries returns supported countries.
func (h *Handler) ListCountries(w http.ResponseWriter, r *http.Request) {
	countries, err := h.service.ListCountries(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"countries": countries})
}

// ListCities returns cities for a country.
func (h *Handler) ListCities(w http.ResponseWriter, r *http.Request) {
	countryIDStr := chi.URLParam(r, "id")
	countryID, err := strconv.ParseInt(countryIDStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("country_id.invalid", "Invalid country ID", nil))
		return
	}

	cities, err := h.service.ListCities(r.Context(), countryID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"cities": cities})
}

// ListCurrencies returns supported currencies.
func (h *Handler) ListCurrencies(w http.ResponseWriter, r *http.Request) {
	currencies, err := h.service.ListCurrencies(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"currencies": currencies})
}

// ListLanguages returns supported languages.
func (h *Handler) ListLanguages(w http.ResponseWriter, r *http.Request) {
	languages, err := h.service.ListLanguages(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"languages": languages})
}

// SubmitContact handles contact form submission.
func (h *Handler) SubmitContact(w http.ResponseWriter, r *http.Request) {
	var m platformadmin.ContactMessage
	if err := httpx.DecodeJSON(w, r, &m); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	if err := h.service.SubmitContactMessage(r.Context(), &m); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, m)
}

// ListContactMessages returns contact inquiries.
func (h *Handler) ListContactMessages(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	messages, err := h.service.ListContactMessages(r.Context(), status, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"messages": messages, "count": len(messages)})
}
