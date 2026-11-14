package main

import (
	"context"
	"log/slog"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/composition/erpports"
	catalog "github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	catalogPostgres "github.com/moqasem061-dot/dawa24/internal/modules/catalog/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	erphttp "github.com/moqasem061-dot/dawa24/internal/modules/erp/http"
	erppostgres "github.com/moqasem061-dot/dawa24/internal/modules/erp/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	inventory "github.com/moqasem061-dot/dawa24/internal/modules/inventory"
	inventoryPostgres "github.com/moqasem061-dot/dawa24/internal/modules/inventory/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/erpgate"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
	"github.com/moqasem061-dot/dawa24/internal/platform/safe"
	"github.com/redis/go-redis/v9"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func mountERP(r chi.Router, cfg *config.Config, log *slog.Logger, deps *dependencies, db *database.DB) *erp.Service {
	erpgate.SetDeployment(cfg.ERP.Enabled)
	// The runtime switch (feature flag erp.enabled) hides the vendor and
	// pharmacy sidebar entries the moment an admin flips it; the staff
	// sidebar keeps its entry so the page stays reachable.
	rbac.NavSuppressedFunc = func(key string) bool {
		return key == "integrations" && !erpgate.Enabled(context.Background())
	}
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
	svc := erp.NewService(repo, erpPorts(db, deps, log), erpCfg, log)
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

// erpPorts fills the sync ports with the same catalog, inventory and matching
// services the website uses (cache and matching wired as routes.go does), so a
// product written by the ERP invalidates the same caches as a website write.
func erpPorts(db *database.DB, deps *dependencies, log *slog.Logger) erp.Ports {
	catSvc := catalog.NewService(catalogPostgres.NewRepository(db), log)
	if cacheHandle := deps.CacheHandle(); cacheHandle != nil {
		catSvc.SetCache(cacheHandle)
	}
	matchSvc := deps.Matching()
	if matchSvc != nil {
		catSvc.SetMatchingService(matchSvc)
	}
	invSvc := inventory.NewService(inventoryPostgres.NewRepository(db), log)
	return erp.Ports{
		VariantWriter: erpports.NewVariantWriter(catSvc),
		StockWriter:   erpports.NewStockWriter(invSvc),
		Matcher:       erpports.NewMatcher(matchSvc),
		MatchMemory:   erpports.NewMatchMemory(matchSvc),
		Enqueuer:      &erpEnqueuer{db: db, log: log},
		Decider:       deps.erpDecide,
		Placer:        deps.erpPurchase,
	}
}

// erpEnqueuer gives the ERP its insert-only River client. The client is built
// on first use: the database handle is not connected while routes are mounted,
// and a failed build is retried on the next call rather than cached.
type erpEnqueuer struct {
	db  *database.DB
	log *slog.Logger

	mu     sync.Mutex
	client *queue.Client
}

// EnqueueTx inserts a job inside the caller's transaction (the sync acceptance
// transaction), so a job that outlives a rolled-back batch can never exist.
func (e *erpEnqueuer) EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	client, err := e.insertClient()
	if err != nil {
		return nil, err
	}
	return client.EnqueueTx(ctx, tx, args, opts)
}

func (e *erpEnqueuer) insertClient() (*queue.Client, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.client != nil {
		return e.client, nil
	}
	client, err := queue.NewInsertOnly(e.db, e.log)
	if err != nil {
		return nil, err
	}
	e.client = client
	return client, nil
}
