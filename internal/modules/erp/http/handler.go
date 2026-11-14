package http

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/erpgate"
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
	// The runtime switch comes first, before the spec and before key
	// authentication: with the integration off, no ERP client learns anything
	// from this API except that it is off.
	r.Use(requireEnabled)
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
		// The order feeds (S3). Sales is the supplier's own shipments; purchase is
		// the buyer's own orders. Acknowledgements take the body through the provider.
		r.Get("/orders/sales", h.listSales)
		r.Get("/orders/sales/{orderNumber}", h.getSale)
		r.Post("/orders/sales/ack", h.ackSales)
		// The supplier's decision on one sales order (S4): confirm or reject.
		r.Post("/orders/sales/{orderNumber}/status", h.decideSale)
		r.Get("/orders/purchase", h.listPurchase)
		r.Get("/orders/purchase/{orderNumber}", h.getPurchase)
		r.Post("/orders/purchase/ack", h.ackPurchase)
		// Purchase orders (S7): the offers lookup and the order creation.
		r.Get("/offers", h.getOffers)
		r.Post("/orders/purchase", h.postPurchase)
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, erp.NewError("ERP_NOT_FOUND", 404, nil))
	})
	return r
}

// requireEnabled answers 503 ERP_DISABLED while an admin has the integration
// switched off. It is evaluated per request, so switching it back on takes
// effect without a restart.
func requireEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !erpgate.Enabled(r.Context()) {
			w.Header().Set("Retry-After", "300")
			writeError(w, r, erp.NewError("ERP_DISABLED", http.StatusServiceUnavailable, nil))
			return
		}
		next.ServeHTTP(w, r)
	})
}
