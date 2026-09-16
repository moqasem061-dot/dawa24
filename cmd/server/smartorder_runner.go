package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/aicapabilities"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder/pipeline"
	smartorderPG "github.com/muhiya/dawa24-store/internal/modules/smartorder/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/progress"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// Running a smart order from the web process.
//
// The feature was originally written to hand the run to the River worker and
// let the buyer watch progress. That is still the right shape when the worker is
// deployed — but when it is not, the run sits in `queued` forever and the buyer
// stares at i18n.TDefault("w4_mod.s_477_477") that never finishes. There is no error to show and
// nothing in the logs, because nothing went wrong: nobody was listening.
//
// So the server processes the run itself, in a goroutine, and the worker stays
// registered for deployments that run it. Exactly one of them does the work:
// the runner claims the run by moving it `queued → processing` and the claim is
// conditional in SQL, so whichever gets there first wins and the other sees zero
// rows affected and stops.
//
// Two details that are easy to get wrong and expensive to debug:
//
//   - The request context must NOT be used. It is cancelled the moment the
//     redirect is written, which would kill the run a few milliseconds in.
//   - A panic in the pipeline must not take the web process down with it.

// smartOrderRunTimeout bounds one run. Generous, because a ten-thousand-row file
// against a thirty-thousand-product catalogue is minutes of honest work — but
// bounded, so a wedged run releases its slot instead of living forever.
const smartOrderRunTimeout = 20 * time.Minute

// inlineSmartOrderRunner processes a run inside the web process.
func inlineSmartOrderRunner(
	db *database.DB,
	orgSvc *org.Service,
	commSvc *commerce.Service,
	ai gateway.Client,
	publisher *progress.Publisher,
	log *slog.Logger,
) ui.SmartOrderEnqueueFunc {
	// The same decorated repository the service uses: this is the half that
	// actually writes the events a watching screen is waiting for.
	repo := smartorder.WithProgressNotifications(smartorderPG.New(db), publisher)

	// AI is optional; a nil enhancer makes the pipeline skip the stage, which
	// is the same path a disabled Gateway takes.
	var enhancer pipeline.Enhancer
	if ai != nil && ai.Enabled() {
		caps := aicapabilities.NewService(ai, log)
		caps.SetKeyResolver(func(ctx context.Context, orgID int64) (string, error) {
			o, err := orgSvc.GetOrganization(ctx, orgID)
			if err != nil || o == nil {
				return "", err
			}
			return o.AIVirtualKey, nil
		})
		enhancer = pipeline.NewGatewayEnhancer(&serverEnhanceAdapter{caps: caps})
	}

	runner := pipeline.NewRunner(repo,
		nil,
		serverInstitutionalGate(orgSvc, log),
		enhancer, log)
	if commSvc != nil {
		runner.SetAvailabilityGate(newCommerceAvailabilityGate(commSvc))
	}

	return func(_ context.Context, runID, orgID int64) error {
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("smart order run panicked", "run_id", runID, "panic", rec)
					_ = repo.UpdateRunStatus(context.Background(), runID,
						smartorder.StatusFailed, 3, "internal error while processing")
				}
			}()

			runCtx := database.WithTenant(context.Background(), orgID)
			runCtx = authctx.ContextWithActor(runCtx, authctx.Actor{
				OrganizationID: orgID,
				OrgID:          orgID,
				Role:           "customer",
			})
			ctx, cancel := context.WithTimeout(runCtx, smartOrderRunTimeout)
			defer cancel()

			if err := executeSmartOrderRun(ctx, repo, runner, orgSvc, runID, orgID, log); err != nil {
				log.ErrorContext(ctx, "smart order run failed", "run_id", runID, "error", err)
				_ = repo.UpdateRunStatus(ctx, runID, smartorder.StatusFailed, 3, err.Error())
			}
		}()
		return nil
	}
}

// executeSmartOrderRun claims the run and drives the pipeline.
func executeSmartOrderRun(
	ctx context.Context,
	repo smartorder.Repository,
	runner *pipeline.Runner,
	orgSvc *org.Service,
	runID, orgID int64,
	log *slog.Logger,
) error {
	run, err := repo.GetRunByID(ctx, orgID, runID)
	if err != nil {
		return err
	}

	// Only a queued run is ours to take. Anything else has been claimed, is
	// already finished, or was reset by the buyer.
	if run.Status != smartorder.StatusQueued {
		log.InfoContext(ctx, "smart order run is not queued; leaving it alone",
			"run_id", runID, "status", run.Status)
		return nil
	}

	cfg, err := repo.GetConfig(ctx, runID)
	if err != nil {
		return err
	}

	branch := pipeline.BranchLocation{BranchID: run.BranchID}
	if b, err := orgSvc.GetBranch(ctx, run.BranchID); err == nil && b != nil &&
		b.Latitude != nil && b.Longitude != nil {
		branch.Lat, branch.Lng, branch.HasCoord = *b.Latitude, *b.Longitude, true
	} else {
		// Worth saying out loud: without coordinates every coverage check passes,
		// so the buyer may be offered suppliers who cannot reach them.
		log.WarnContext(ctx, "branch has no coordinates; coverage cannot be enforced",
			"run_id", runID, "branch_id", run.BranchID)
	}

	if claimer, ok := repo.(smartorder.RunClaimer); ok {
		claimed, err := claimer.ClaimRun(ctx, orgID, runID)
		if err != nil {
			return err
		}
		if !claimed {
			log.InfoContext(ctx, "smart order run claim lost", "run_id", runID)
			return nil
		}
		run.Status = smartorder.StatusProcessing
	} else {
		if err := run.TransitionTo(smartorder.StatusProcessing); err != nil {
			return err
		}
		if err := repo.UpdateRunStatus(ctx, run.ID, run.Status, run.CurrentStep, ""); err != nil {
			return err
		}
	}

	if err := runner.Execute(ctx, run, cfg, branch); err != nil {
		return err
	}

	if err := run.TransitionTo(smartorder.StatusCompleted); err != nil {
		return err
	}
	return repo.UpdateRunStatus(ctx, run.ID, run.Status, run.CurrentStep, "")
}

// serverInstitutionalGate applies Corporate Operations — the platform's rule,
// the one checkout applies.
//
// It used to apply a different one: it intersected the PRODUCT's
// institutional_work_ids with the buyer organisation's employee work
// assignments. That is not the question commerce.CheckAvailability asks, so a
// run's review screen showed lines as orderable that checkout then refused, and
// the buyer found out at the last click — on the whole order at once, with no
// line named and nothing to fix. The rule now comes from one implementation,
// org.Service.BranchesInstitutionallyConnected, which is the same call the
// availability probe behind CheckAvailability makes.
//
// It fails CLOSED, unlike the version before it. Letting an offer through on a
// lookup error is not a kindness when checkout will refuse it anyway: it moves
// the refusal from a named line on the review screen to a dead end at the end.
func serverInstitutionalGate(svc *org.Service, log *slog.Logger) pipeline.InstitutionalGate {
	if svc == nil {
		return smartorder.AlwaysInstitutionallyVisible()
	}
	return smartorder.InstitutionalFunc(func(ctx context.Context, c smartorder.InstitutionalCheck) (bool, error) {
		// AsSystem for the same reason the availability probe uses it: the
		// buyer is reading another tenant's branches to answer "may I buy
		// this", and RLS would otherwise return nothing and refuse everyone.
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

// serverEnhanceAdapter bridges aicapabilities to the pipeline's gateway contract.
//
// The translation is mechanical and lives here rather than in either module,
// because this is the only place allowed to know about both.
type serverEnhanceAdapter struct {
	caps *aicapabilities.Service
}

func (b *serverEnhanceAdapter) EnhanceBatch(ctx context.Context, batch pipeline.GatewayBatch) ([]pipeline.GatewayOutcome, error) {
	req := aicapabilities.EnhanceRequest{
		Catalog: make([]aicapabilities.CatalogEntry, 0, len(batch.Catalog)),
		Items:   make([]aicapabilities.EnhanceItem, 0, len(batch.Items)),
		// Attribution for the AI usage ledger: the pharmacy reads i18n.TDefault("w4_mod.s_478_478") on its usage log, not a capability name.
		Feature: matchflow.FeatureSmartOrder,
	}
	for _, c := range batch.Catalog {
		req.Catalog = append(req.Catalog, aicapabilities.CatalogEntry(c))
	}
	for _, it := range batch.Items {
		req.Items = append(req.Items, aicapabilities.EnhanceItem(it))
	}
	if tid, ok := database.TenantFrom(ctx); ok && tid > 0 {
		req.OrganizationID = tid
	}
	if actor, ok := authctx.From(ctx); ok {
		if req.OrganizationID <= 0 {
			req.OrganizationID = actor.OrganizationID
		}
		req.UserID = actor.UserID
	}

	decisions, err := b.caps.EnhanceMatches(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make([]pipeline.GatewayOutcome, 0, len(decisions))
	for _, d := range decisions {
		out = append(out, pipeline.GatewayOutcome(d))
	}
	return out, nil
}
