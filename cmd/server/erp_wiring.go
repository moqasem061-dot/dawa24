package main

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	erphttp "github.com/moqasem061-dot/dawa24/internal/modules/erp/http"
	erppostgres "github.com/moqasem061-dot/dawa24/internal/modules/erp/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
	"github.com/moqasem061-dot/dawa24/internal/platform/safe"
	"github.com/redis/go-redis/v9"
)

func mountERP(r chi.Router, cfg *config.Config, log *slog.Logger, deps *dependencies, db *database.DB) *erp.Service {
	if !cfg.ERP.Enabled {
		// The integrations screens' routes, modals and sidebar entries come
		// from the RPC-facing half of the slice; with the API off the page
		// cannot do anything useful, so its routes are not registered and its
		// sidebar entries are hidden. This is a deployment switch: the
		// permission gates stay the control whenever it is on.
		rbac.NavSuppressions["integrations"] = true
		return nil
	}
	tag := "test"
	if cfg.Env.IsProd() {
		tag = "live"
	}
	erpCfg := erp.Config{Pepper: []byte(cfg.ERP.KeyPepper), Tag: tag, DefaultRate: cfg.ERP.DefaultRate, MaxBatchItems: cfg.ERP.MaxBatchItems, Provider: provider.DefaultProvider}
	repo := erppostgres.New(db)
	svc := erp.NewService(repo, erp.Ports{}, erpCfg, log)
	lim := httpx.NewLazyLimiter(func() *redis.Client {
		if deps.CacheHandle() == nil {
			return nil
		}
		return deps.CacheHandle().Redis()
	}, "dawa24:erp:ratelimit:")
	calls := erp.NewCallLogger(repo, log)
	safe.Go(log, "erp-call-log", func() { calls.Run(deps.Context()) })
	r.Mount("/erp/v1", erphttp.NewHandler(svc, erpCfg, lim, log).Routes())
	return svc
}
