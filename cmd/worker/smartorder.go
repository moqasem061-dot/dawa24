package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/moqasem061-dot/dawa24/internal/modules/aicapabilities"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	catalogPG "github.com/moqasem061-dot/dawa24/internal/modules/catalog/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/commerce"
	commercePG "github.com/moqasem061-dot/dawa24/internal/modules/commerce/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/inventory"
	inventoryPG "github.com/moqasem061-dot/dawa24/internal/modules/inventory/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/org"
	orgPG "github.com/moqasem061-dot/dawa24/internal/modules/org/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder"
	smartorderJobs "github.com/moqasem061-dot/dawa24/internal/modules/smartorder/jobs"
	"github.com/moqasem061-dot/dawa24/internal/modules/smartorder/pipeline"
	smartorderPG "github.com/moqasem061-dot/dawa24/internal/modules/smartorder/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/workflow"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/gateway"
)

// registerSmartOrderWorker wires the smart ordering pipeline into the worker.
//
// The pipeline's dependencies arrive as narrow interfaces so the module never
// imports workflow, org or the gateway (AGENTS.md rule 5 and rule 2). The
// adaptation happens here, in the composition root, where knowing about all of
// them is the job.
func registerSmartOrderWorker(
	workers *river.Workers,
	db *database.DB,
	ai gateway.Client,
	log *slog.Logger,
) {
	repo := smartorderPG.New(db)
	coverage := workflow.NewCoverageService(db)
	orgSvc := org.NewService(orgPG.NewRepository(db), log)
	catSvc := catalog.NewService(catalogPG.NewRepository(db), log)
	invSvc := inventory.NewService(inventoryPG.NewRepository(db), log)
	commRepo := commercePG.NewRepository(db)
	commSvc := commerce.NewService(commRepo, log)
	commSvc.SetAvailabilityProbe(newWorkerAvailabilityProbe(catSvc, orgSvc, coverage, invSvc))

	// AI is optional. A nil enhancer makes the pipeline skip the stage
	// entirely, which is the same path a disabled Gateway takes — the run
	// completes on deterministic results (AGENTS.md rule 3).
	var enhancer pipeline.Enhancer
	if ai != nil && ai.Enabled() {
		caps := aicapabilities.NewService(ai, log)
		caps.SetKeyResolver(smartOrderKeyResolver(orgSvc))
		enhancer = caps
	}

	runner := pipeline.NewRunner(
		repo,
		nil,
		institutionalGate(orgSvc, log),
		enhancer,
		log,
	)
	runner.SetAvailabilityGate(newWorkerAvailabilityGate(commSvc))

	river.AddWorker(workers, smartorderJobs.NewRunWorker(
		repo, runner, branchResolver(orgSvc), log,
	))
}

type workerAvailabilityGate struct {
	commSvc *commerce.Service
}

func newWorkerAvailabilityGate(commSvc *commerce.Service) smartorder.AvailabilityGate {
	return &workerAvailabilityGate{commSvc: commSvc}
}

func (g *workerAvailabilityGate) Check(ctx context.Context, buyerOrgID, buyerBranchID int64, when time.Time,
	lines []smartorder.GateLine) (map[int64]smartorder.GateVerdict, error) {
	if g.commSvc == nil {
		verdicts := make(map[int64]smartorder.GateVerdict, len(lines))
		for _, l := range lines {
			verdicts[l.VariantID] = smartorder.GateVerdict{
				Allowed: false,
				Reason:  "variant_invalid",
			}
		}
		return verdicts, nil
	}
	cLines := make([]commerce.AvailabilityLine, len(lines))
	for i, l := range lines {
		cLines[i] = commerce.AvailabilityLine{
			VariantID:   l.VariantID,
			VendorOrgID: l.VendorOrgID,
			Quantity:    l.Quantity,
		}
	}
	results, err := g.commSvc.CheckAvailabilityBatch(ctx, buyerOrgID, buyerBranchID, when, cLines)
	if err != nil {
		return nil, err
	}
	verdicts := make(map[int64]smartorder.GateVerdict, len(results))
	for vid, r := range results {
		verdicts[vid] = smartorder.GateVerdict{
			Allowed:     r.Allowed,
			MaxQuantity: r.MaxQuantity,
			Reason:      string(r.Reason),
		}
	}
	return verdicts, nil
}

// institutionalGate adapts Corporate Operations — the platform's branch-to-branch
// rule, which is the one commerce.CheckAvailability applies at checkout.
//
// The worker and the web process must decide this identically or the same file
// produces different results depending on which one happened to pick it up, so
// both call org.Service.BranchesInstitutionallyConnected and neither carries a
// copy of the rule. See cmd/server/smartorder_runner.go for why it fails closed.
func institutionalGate(svc *org.Service, log *slog.Logger) pipeline.InstitutionalGate {
	if svc == nil {
		return smartorder.AlwaysInstitutionallyVisible()
	}
	return smartorder.InstitutionalFunc(func(ctx context.Context, c smartorder.InstitutionalCheck) (bool, error) {
		ok, err := svc.BranchesInstitutionallyConnected(database.AsSystem(ctx), org.InstitutionalConnection{
			BuyerBranchID:  c.BuyerBranchID,
			VendorOrgID:    c.VendorOrgID,
			VendorBranchID: c.VendorBranchID,
		})
		if err != nil {
			log.WarnContext(ctx, "institutional connection lookup failed; treating the offer as restricted",
				"buyer_branch_id", c.BuyerBranchID, "vendor_org_id", c.VendorOrgID, "error", err)
			return false, nil
		}
		return ok, nil
	})
}

// branchResolver supplies the delivery branch's coordinates.
func branchResolver(svc *org.Service) smartorderJobs.BranchResolver {
	return smartorder.BranchLocationFunc(func(ctx context.Context, orgID, branchID int64) (float64, float64, bool, error) {
		b, err := svc.GetBranch(ctx, branchID)
		if err != nil || b == nil {
			return 0, 0, false, err
		}
		if b.Latitude == nil || b.Longitude == nil {
			return 0, 0, false, nil
		}
		return *b.Latitude, *b.Longitude, true, nil
	})
}

// smartOrderKeyResolver bills adjudication to the buyer's organisation.
//
// Gateway identity is per organisation, not per user: that is how the platform
// provisions it, and it means one buyer's imports share one budget.
func smartOrderKeyResolver(svc *org.Service) aicapabilities.KeyResolver {
	return func(ctx context.Context, orgID int64) (string, error) {
		o, err := svc.GetOrganization(ctx, orgID)
		if err != nil || o == nil {
			return "", err
		}
		return o.AIVirtualKey, nil
	}
}
