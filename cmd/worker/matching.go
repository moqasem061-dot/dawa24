package main

import (
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/moqasem061-dot/dawa24/internal/modules/aicapabilities"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	matchingJobs "github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching/jobs"
	matchingPostgres "github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
	smartorderPipeline "github.com/moqasem061-dot/dawa24/internal/modules/smartorder/pipeline"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/gateway"
	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// registerMatchingWorkers wires match reaper, enhancement, and purge workers and returns the
// matching service (for the ERP ports) and the periodic jobs.
func registerMatchingWorkers(
	workers *river.Workers,
	db *database.DB,
	ai gateway.Client,
	log *slog.Logger,
) (matching.Service, []*river.PeriodicJob) {
	matchingRepo := matchingPostgres.NewRepository(db)
	matchingCuratedRepo := matchingPostgres.NewCuratedRepository(db)
	matchingLoader := matching.NewLoader(db)
	matchingCache := matching.NewIndexCache(matching.NewDefaultIndexBuilderWithCuration(matchingLoader, matchingCuratedRepo))
	matchingSvc := matching.NewService(matchingRepo, matchingCache, matchingLoader)

	// Register the 5 consumer Appliers (T127)
	matchingSvc.RegisterApplier(matching.WorkflowVendor, ingest.NewCatalogAIApplier(db))
	matchingSvc.RegisterApplier(matching.WorkflowCompare, compare.NewCompareAIApplier(db))
	matchingSvc.RegisterApplier(matching.WorkflowSmartOrder, smartorderPipeline.NewSmartOrderAIApplier(db))
	matchingSvc.RegisterApplier(matching.WorkflowCatalog, catalog.NewCatalogImportAIApplier(db))
	matchingSvc.RegisterApplier(matching.WorkflowSavings, catalog.NewSavingAIApplier(db))

	// Reaping stale match runs (US12 T103)
	river.AddWorker(workers, matchingJobs.NewMatchReaperWorker(db, log))

	// AI Enhancement worker (US9 T125)
	var aiEnhancer matchflow.Enhancer
	if ai != nil && ai.Enabled() {
		aiEnhancer = aicapabilities.NewService(ai, log)
	}
	river.AddWorker(workers, matchingJobs.NewEnhanceWorker(matchingSvc, matchingRepo, matchingCache, aiEnhancer, nil, log))

	// Shadow Matching worker (US13 T151)
	river.AddWorker(workers, matching.NewShadowWorker(matchingSvc, log))

	// Attempt Purge worker (US9 T129)
	river.AddWorker(workers, matchingJobs.NewPurgeAttemptsWorker(matchingRepo, log))

	reaperJob := river.NewPeriodicJob(
		river.PeriodicInterval(15*time.Second),
		func() (river.JobArgs, *river.InsertOpts) {
			return queue.MatchReaperArgs{}, nil
		},
		nil,
	)

	purgeJob := river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return queue.MatchPurgeAttemptsArgs{}, nil
		},
		nil,
	)

	return matchingSvc, []*river.PeriodicJob{reaperJob, purgeJob}
}
