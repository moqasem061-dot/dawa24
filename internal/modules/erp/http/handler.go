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
	r.Get("/openapi.yaml", h.spec)
	r.Group(func(r chi.Router) { r.Use(h.auth); r.Get("/health", h.health) })
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, erp.NewError("ERP_NOT_FOUND", 404, nil))
	})
	return r
}
