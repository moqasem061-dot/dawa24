package http

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
)

type Handler struct {
	svc   *erp.Service
	cfg   erp.Config
	lim   *httpx.Limiter
	log   *slog.Logger
	calls *erp.CallLogger
}

func NewHandler(svc *erp.Service, cfg erp.Config, lim *httpx.Limiter, log *slog.Logger) *Handler {
	return &Handler{svc: svc, cfg: cfg, lim: lim, log: log}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	// The spec stays public (S6/S8 ship it as documentation); it carries no
	// tenant data, but it is still unauthenticated output, so it gets its own
	// per-IP counter — separate from the per-key limit and the auth-failure
	// counter, and from any website limiter.
	r.With(h.docsLimit).Get("/openapi.yaml", h.spec)
	r.Group(func(r chi.Router) {
		r.Use(h.auth)
		r.Get("/health", h.health)
		// The product-sync surface (S2): all three need the authenticated
		// principal; the scope check lives in AcceptBatch, and the status/item
		// reads are tenant-scoped by the key's organization.
		r.Post("/products/sync", h.postSync)
		r.Get("/products/sync/{syncId}", h.getSync)
		r.Get("/products/sync/{syncId}/items", h.getSyncItems)
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, erp.NewError("ERP_NOT_FOUND", 404, nil))
	})
	return r
}
