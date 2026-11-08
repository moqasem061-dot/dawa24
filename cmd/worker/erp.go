package main

import (
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/moqasem061-dot/dawa24/internal/composition/erpports"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	matching "github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	catalogpg "github.com/moqasem061-dot/dawa24/internal/modules/catalog/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/jobs"
	erppg "github.com/moqasem061-dot/dawa24/internal/modules/erp/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/modules/inventory"
	inventorypg "github.com/moqasem061-dot/dawa24/internal/modules/inventory/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
)

// registerERPWorkers wires the ERP sync chunk worker and the nightly retention
// sweep onto the worker binary. The two processes need the same four sibling
// writes, so the composition adapters (ADR-ERP-1) adapt the catalog, inventory
// and matching services into the ERP's ports. The worker never mints keys, so
// it needs no pepper — the pepper check happened at config load, and the
// worker's erp.Config is used only for defaults of new rows and the provider
// registry key. Returns the registered-worker count (2) and the periodic jobs,
// so the wiring gate (cfg.ERP.Enabled) is testable without river internals.
func registerERPWorkers(
	workers *river.Workers,
	db *database.DB,
	matchSvc matching.Service,
	log *slog.Logger,
) (int, []*river.PeriodicJob) {
	repo := erppg.New(db)
	catSvc := catalog.NewService(catalogpg.NewRepository(db), log)
	invSvc := inventory.NewService(inventorypg.NewRepository(db), log)

	svc := erp.NewService(repo, erp.Ports{
		VariantWriter: erpports.NewVariantWriter(catSvc),
		StockWriter:   erpports.NewStockWriter(invSvc),
		Matcher:       erpports.NewMatcher(matchSvc),
		MatchMemory:   erpports.NewMatchMemory(matchSvc),
		// Enqueuer stays nil on the worker: acceptance happens in the web
		// process; the worker only drains existing chunks.
	}, erp.Config{
		Pepper:        nil,
		Tag:           "live",
		DefaultRate:   erp.DefaultRatePerMinute,
		MaxBatchItems: erp.MaxBatchItems,
		Provider:      provider.DefaultProvider,
	}, log)

	river.AddWorker(workers, jobs.NewSyncChunkWorker(svc, log))
	river.AddWorker(workers, jobs.NewCleanupWorker(svc, log))

	cleanupJob := river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return queue.ERPCleanupArgs{}, &river.InsertOpts{Queue: "maintenance", MaxAttempts: 1}
		},
		nil,
	)
	return 2, []*river.PeriodicJob{cleanupJob}
}
