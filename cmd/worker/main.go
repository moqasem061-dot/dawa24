// Command worker runs background jobs across separated queues (notifications, imports, maintenance).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/riverqueue/river"

	"github.com/moqasem061-dot/dawa24/internal/modules/billing"
	billingPostgres "github.com/moqasem061-dot/dawa24/internal/modules/billing/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/importjobs"
	catalogJobs "github.com/moqasem061-dot/dawa24/internal/modules/catalog/jobs"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	matchingJobs "github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching/jobs"
	matchingPostgres "github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching/postgres"
	catalogPostgres "github.com/moqasem061-dot/dawa24/internal/modules/catalog/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	comparePostgres "github.com/moqasem061-dot/dawa24/internal/modules/compare/postgres"
	ingestPostgres "github.com/moqasem061-dot/dawa24/internal/modules/ingest/postgres"
	platformadmin "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin"
	platformadminPostgres "github.com/moqasem061-dot/dawa24/internal/modules/platform_admin/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/promo"
	promoPostgres "github.com/moqasem061-dot/dawa24/internal/modules/promo/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/aiusage"
	aiusagePostgres "github.com/moqasem061-dot/dawa24/internal/platform/aiusage/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/cache"
	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/gateway"
	"github.com/moqasem061-dot/dawa24/internal/platform/importrun"
	importrunPostgres "github.com/moqasem061-dot/dawa24/internal/platform/importrun/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/observability"
	"github.com/moqasem061-dot/dawa24/internal/platform/progress"
	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
	"github.com/moqasem061-dot/dawa24/internal/platform/safe"
	"github.com/moqasem061-dot/dawa24/internal/platform/storage"
	_ "github.com/moqasem061-dot/dawa24/internal/shared/timeutil"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := observability.NewLogger(cfg.Observ, cfg.Env)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The worker gets its own statement ceiling.
	//
	// statement_timeout is a RuntimeParam on every connection in the pool, and
	// this process opens its pool from the same Database config the web process
	// does — so the web's thirty-second ceiling silently became the ceiling on
	// every statement inside a background import too, however long River's
	// JobTimeout was. A bulk write of a large catalogue was cancelled by
	// Postgres mid-job.
	workerDB := cfg.Database
	if workerDB.WorkerStatementTimeout > 0 {
		workerDB.StatementTimeout = workerDB.WorkerStatementTimeout
	}
	db, err := database.Open(ctx, workerDB)
	if err != nil {
		return err
	}
	log.Info("worker database ready", "statement_timeout", workerDB.StatementTimeout)
	defer db.Close()

	var s3Store *storage.Client
	if sc, err := storage.New(ctx, cfg.Storage); err == nil {
		s3Store = sc
	}

	workers := river.NewWorkers()
	river.AddWorker(workers, &heartbeatWorker{log: log})
	river.AddWorker(workers, &orderNotificationWorker{db: db, log: log})
	river.AddWorker(workers, &notificationDeliverWorker{db: db, log: log})
	river.AddWorker(workers, &ingestBatchWorker{db: db, log: log})
	river.AddWorker(workers, &expirePromotionsWorker{db: db, log: log, s3: s3Store})
	river.AddWorker(workers, catalogJobs.NewProductReindexWorker(db, log))
	river.AddWorker(workers, newDocumentScanWorker(db, s3Store, log))

	// Smart ordering (specs/001-smart-ordering-system). Registered with AI Gateway
	// client if configured, or deterministic fallback if unconfigured.
	// The same usage ledger the web process writes. A smart order run or an
	// import that AI settles happens here, not in a request, and consumption
	// recorded only on the web side would miss most of what a tenant spends.
	usageRecorder := aiusage.NewRecorder(aiusagePostgres.NewRepository(db), log)
	defer func() {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer flushCancel()
		usageRecorder.Close(flushCtx)
	}()

	var ai gateway.Client
	if cfg.Gateway.BaseURL != "" || cfg.Gateway.ClientApp != "" {
		ai = gateway.WithUsageRecorder(gateway.New(cfg.Gateway, log), usageRecorder)
	}
	registerSmartOrderWorker(workers, db, ai, log)

	// Unified import workers (Task 18). Stage parses and matches the uploaded
	// file; commit persists the reviewed rows into their destination tables.
	// Both run from the "imports" queue, which cfg.Worker.Queues already
	// provisions with 2 workers.
	// Live progress out of the worker.
	//
	// An import runs here and the person watching it is on the web process, so
	// without a channel between them every bar in the product falls back to
	// asking the database twice a second. Redis is that channel; when it is not
	// reachable the worker simply does not announce, the browser keeps polling,
	// and nothing breaks — which is the same trade the cache makes everywhere
	// else on this platform.
	var progressCache *cache.Cache
	if opened, cacheErr := cache.Open(ctx, cfg.Redis, cfg.Env); cacheErr == nil {
		progressCache = opened
		defer func() { _ = progressCache.Close() }()
	} else {
		log.Warn("progress channel unavailable; import bars will poll", "error", cacheErr)
	}
	importRunRepo := importrun.WithProgress(
		importrunPostgres.New(db),
		progress.NewPublisher(func() *redis.Client {
			if progressCache == nil {
				return nil
			}
			return progressCache.Redis()
		}, progress.NewHub(), log),
	)
	catRepo := catalogPostgres.NewRepository(db)
	catSvcWorker := catalog.NewService(catRepo, log)
	river.AddWorker(workers, importjobs.NewStageWorker(db, importRunRepo, catSvcWorker, log))
	river.AddWorker(workers, importjobs.NewCommitWorker(db, importRunRepo, catSvcWorker, log))

	matchingRepo := matchingPostgres.NewRepository(db)
	matchingCuratedRepo := matchingPostgres.NewCuratedRepository(db)
	matchingLoader := matching.NewLoader(db)
	matchingCache := matching.NewIndexCache(matching.NewDefaultIndexBuilderWithCuration(matchingLoader, matchingCuratedRepo))
	matchingSvc := matching.NewService(matchingRepo, matchingCache, matchingLoader)
	_ = matchingSvc

	river.AddWorker(workers, matchingJobs.NewMatchReaperWorker(db, log))

	reaperPeriodicJob := river.NewPeriodicJob(
		river.PeriodicInterval(15*time.Second),
		func() (river.JobArgs, *river.InsertOpts) {
			return queue.MatchReaperArgs{}, nil
		},
		nil,
	)

	queueClient, err := queue.New(db, workers, cfg.Worker, log, reaperPeriodicJob)
	if err != nil {
		return err
	}
	// Normally already applied by `cli migrate`; repeated here so a deploy
	// whose migrate step could not reach the database still gets its queue.
	if err := queue.MigrateSchema(ctx, db); err != nil {
		log.Warn("job queue migrations note", "error", err)
	}

	if err := queueClient.Start(ctx); err != nil {
		return err
	}

	// Releasing imports wedged in 'processing'.
	//
	// A staging run lives in the web process, so a deploy or a crash strands the
	// session it was working on: the vendor sees a progress bar that polls for
	// ever against work nobody is doing. Nothing swept those up, and the only
	// recovery was editing the row by hand. A session whose rows are staged is
	// promoted to review with its counters recomputed — the vendor gets the
	// result the dead process already produced — and one with no rows is failed
	// so they are told to upload again.
	safe.Go(log, "worker-wedged-import-sweep", func() {
		importRepo := ingestPostgres.NewRepository(db)
		sweep := func(reason string) {
			// Cross-tenant by nature: it sweeps every organisation's sessions.
			n, err := importRepo.RecoverStaleRuns(database.AsSystem(ctx))
			if err != nil {
				log.Error("wedged import sweep failed", "trigger", reason, "error", err)
				return
			}
			if n > 0 {
				log.Info("released wedged import runs", "trigger", reason, "count", n)
			}
		}

		// Once shortly after boot, because a crash loop is exactly when
		// sessions get stranded and exactly when somebody is watching.
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
			sweep("startup")
		}

		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweep("periodic")
			}
		}
	})

	// Daily Subscription Renewal Scheduler (Runs once every 24 hours)
	safe.Go(log, "worker-subscription-renewals", func() {
		billSvc := billing.NewService(billingPostgres.NewRepository(db), log)
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
			if r, f, err := billSvc.ProcessDueSubscriptionRenewals(ctx); err != nil {
				log.Error("initial subscription renewal pass failed", "error", err)
			} else if r > 0 || f > 0 {
				log.Info("initial subscription renewal pass completed", "renewed", r, "failed", f)
			}
		}

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if r, f, err := billSvc.ProcessDueSubscriptionRenewals(ctx); err != nil {
					log.Error("daily subscription renewal pass failed", "error", err)
				} else {
					log.Info("daily subscription renewal pass completed", "renewed", r, "failed", f)
				}
			}
		}
	})

	// Periodic Compare Files & Temporary Warehouse Lifecycle Scheduler (Runs every 1 hour)
	safe.Go(log, "worker-warehouse-lifecycle", func() {
		compareRepo := comparePostgres.NewRepository(db)
		compareSvc := compare.NewService(compareRepo, log)
		adminRepo := platformadminPostgres.NewRepository(db)
		adminSvc := platformadmin.NewService(adminRepo, log)

		runLifecyclePass := func(trigger string) {
			sysCtx := database.AsSystem(ctx)
			settings, err := adminSvc.GetTempWarehouseLifecycleSettings(sysCtx)
			if err != nil {
				log.Warn("could not load temp warehouse lifecycle settings, using defaults", "error", err)
			}
			autoArchiveHours := 720
			autoArchiveEnabled := true
			autoDeleteDays := 30
			autoDeleteEnabled := true
			if settings != nil {
				autoArchiveHours = settings.AutoArchiveHours
				autoArchiveEnabled = settings.AutoArchiveEnabled
				autoDeleteDays = settings.AutoDeleteDays
				autoDeleteEnabled = settings.AutoDeleteEnabled
			}

			res, err := compareSvc.RunWarehouseLifecycle(
				sysCtx,
				autoArchiveHours,
				autoArchiveEnabled,
				autoDeleteDays,
				autoDeleteEnabled,
				storage.UploadBaseDir(),
			)
			if err != nil {
				log.Error("warehouse & compare files lifecycle pass failed", "trigger", trigger, "error", err)
				return
			}
			if res != nil && (res.ArchivedFilesCount > 0 || res.DeletedFilesCount > 0 || res.PurgedRowsCount > 0) {
				log.Info("warehouse & compare files lifecycle pass completed",
					"trigger", trigger,
					"archived_files", res.ArchivedFilesCount,
					"deleted_files", res.DeletedFilesCount,
					"purged_rows", res.PurgedRowsCount,
				)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(20 * time.Second):
			runLifecyclePass("initial")
		}

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runLifecyclePass("periodic")
			}
		}
	})

	// Periodic Promotions & Media Expiry Scheduler (Runs every 15 minutes)
	safe.Go(log, "worker-promotions-sweeper", func() {
		promoSvc := promo.NewService(promoPostgres.NewRepository(db), log)

		sweep := func(trigger string) {
			sysCtx := database.AsSystem(ctx)
			if exp, purged, err := promoSvc.ExpirePromotionsWithMediaPurge(sysCtx, s3Store); err != nil {
				log.Error("promotions and media expiry sweep failed", "trigger", trigger, "error", err)
			} else if exp > 0 || purged > 0 {
				log.Info("promotions and media expiry sweep completed",
					"trigger", trigger, "expired_promotions", exp, "purged_media", purged)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(15 * time.Second):
			sweep("startup")
		}

		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweep("periodic")
			}
		}
	})

	// Capsule retention: conversations are deleted six months after they were
	// created, and unreferenced uploads after a day. See assistant_retention.go.
	startAssistantRetention(ctx, db, cfg.Storage, log)

	log.Info("worker started", "queues", cfg.Worker.Queues)

	<-ctx.Done()
	log.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Worker.ShutdownTimeout)
	defer cancel()

	if err := queueClient.Stop(shutdownCtx); err != nil {
		log.Error("graceful worker shutdown failed", "error", err)
		return err
	}

	log.Info("worker shutdown complete")
	return nil
}
