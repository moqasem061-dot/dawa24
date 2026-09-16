package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/aicapabilities"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder/pipeline"
	smartorderPG "github.com/muhiya/dawa24-store/internal/modules/smartorder/postgres"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// smartOrderSmoke drives a real run end to end against the live catalogue.
//
// This is the check that matters before anyone trusts the feature: it proves
// the deterministic ladder actually resolves this catalogue's products, and it
// reports the funnel — how many rows each tier settled — so a bad tier is
// visible rather than hidden behind an aggregate.
//
// It drives the AI enhancement stage too, whenever the Gateway is configured.
// That stage is the one whose failures are invisible in production — a run
// completes either way — so being able to see its funnel from a terminal is the
// difference between "AI is enabled" and "AI is doing something".
//
//	cli smartorder-smoke <orgID> <branchID> <userID> <file.xlsx>
func smartOrderSmoke(ctx context.Context, db *database.DB, gw gateway.Client,
	orgSvc *org.Service, log *slog.Logger, args []string) error {
	if len(args) < 4 {
		return fmt.Errorf("usage: cli smartorder-smoke <orgID> <branchID> <userID> <file>")
	}
	orgID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("orgID: %w", err)
	}
	branchID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return fmt.Errorf("branchID: %w", err)
	}
	userID, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return fmt.Errorf("userID: %w", err)
	}
	path := args[3]

	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	repo := smartorderPG.New(db)
	svc := smartorder.NewService(repo, log)

	fmt.Println("== step 1: inspect the file ==")
	parsed, err := pipeline.Inspect(content, path)
	if err != nil {
		return fmt.Errorf("inspect: %w", err)
	}
	fmt.Printf("   header row %d, %d columns\n", parsed.HeaderRow, len(parsed.Headers))
	for col, field := range parsed.Detected {
		fmt.Printf("   column %d (%q) -> %s  [%.0f%%]\n",
			col, parsed.Headers[col], field, parsed.Confidence[field]*100)
	}

	fmt.Println("== step 2: create the run ==")
	useAI := gw != nil && gw.Enabled()
	run, err := svc.Start(ctx, smartorder.StartOptions{
		UserID: userID, OrganizationID: orgID, BranchID: branchID,
		Filename: path, UseSavingProducts: true, UseAIMatching: useAI,
	})
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	fmt.Printf("   %s (%s)\n", run.RunNumber, run.PublicID)

	cfg, err := svc.Config(ctx, run.ID)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	m := &smartorder.Mapping{RunID: run.ID, OrganizationID: orgID, HeaderRow: parsed.HeaderRow, Fields: parsed.Detected}
	if err := svc.ConfirmMapping(ctx, run, m); err != nil {
		return fmt.Errorf("mapping: %w", err)
	}

	fmt.Println("== step 3: stage the rows ==")
	lines, err := pipeline.Stage(content, path, m, run.ID, orgID)
	if err != nil {
		return fmt.Errorf("stage: %w", err)
	}
	if err := svc.StageLines(ctx, lines); err != nil {
		return fmt.Errorf("insert lines: %w", err)
	}
	fmt.Printf("   %d rows staged\n", len(lines))

	fmt.Println("== step 4: run the pipeline ==")
	if err := svc.Queue(ctx, run); err != nil {
		return fmt.Errorf("queue: %w", err)
	}
	if err := run.TransitionTo(smartorder.StatusProcessing); err != nil {
		return err
	}
	_ = repo.UpdateRunStatus(ctx, run.ID, run.Status, run.CurrentStep, "")

	branch := pipeline.BranchLocation{BranchID: branchID}
	if b, err := orgSvc.GetBranch(ctx, branchID); err == nil && b != nil &&
		b.Latitude != nil && b.Longitude != nil {
		branch.Lat, branch.Lng, branch.HasCoord = *b.Latitude, *b.Longitude, true
	}
	fmt.Printf("   branch coordinates: %v\n", branch.HasCoord)

	fmt.Printf("   AI enhancement: %v\n", useAI)
	runner := pipeline.NewRunner(repo,
		coverageGateCLI(workflow.NewCoverageService(db)),
		smartorder.SimpleInstitutionalGate(nil),
		smokeEnhancer(gw, orgSvc, log),
		log)

	if err := runner.Execute(ctx, run, cfg, branch); err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}

	fmt.Println("== results ==")
	fmt.Printf("   total          %d\n", run.Stats.TotalRows)
	fmt.Printf("   matched        %d  (%.0f%%)\n", run.Stats.MatchedRows,
		pct(run.Stats.MatchedRows, run.Stats.TotalRows))
	fmt.Printf("   unmatched      %d\n", run.Stats.UnmatchedRows)
	fmt.Printf("   no supplier    %d\n", run.Stats.NoSupplierRows)
	fmt.Printf("   coverage       %d\n", run.Stats.CoverageBlockedRows)
	fmt.Printf("   institutional  %d\n", run.Stats.InstitutionalBlockedRows)
	fmt.Printf("   quota blocked  %d\n", run.Stats.QuotaBlockedRows)
	fmt.Printf("   below min qty  %d\n", run.Stats.BelowMinQtyRows)
	fmt.Printf("   estimated cost %s\n", run.EstimatedTotal.String())
	if run.DeterministicMS != nil {
		fmt.Printf("   deterministic  %d ms\n", *run.DeterministicMS)
	}
	if run.TotalMS != nil {
		fmt.Printf("   total          %d ms\n", *run.TotalMS)
	}

	if run.AI.Enabled {
		fmt.Println("== AI enhancement ==")
		fmt.Printf("   reviewed       %d\n", run.AI.LinesReviewed)
		fmt.Printf("   from cache     %d\n", run.AI.CacheHits)
		fmt.Printf("   requests       %d\n", run.AI.Calls)
		fmt.Printf("   answered       %d\n", run.AI.LinesAdjudicated)
		fmt.Printf("   improved       %d\n", run.AI.LinesImproved)
		fmt.Printf("   ceiling hit    %v\n", run.AI.CeilingHit)
	}

	fmt.Println("== which tier settled each row ==")
	byMethod := map[smartorder.MatchMethod]int{}
	stored, _, err := repo.ListLines(ctx, run.ID, smartorder.LineFilter{All: true})
	if err != nil {
		return err
	}
	for _, l := range stored {
		byMethod[l.MatchMethod]++
	}
	for method, n := range byMethod {
		fmt.Printf("   %-16s %d\n", method, n)
	}

	fmt.Println("== a few rows ==")
	for i, l := range stored {
		if i >= 12 {
			break
		}
		product := "—"
		if l.Matched() {
			product = fmt.Sprintf("#%d", *l.MatchedProductID)
		}
		fmt.Printf("   %-38.38s -> %-10s %-16s %3.0f%%  %s\n",
			l.RawName, product, l.MatchMethod, l.MatchConfidence*100, l.Outcome)
	}

	_ = run.TransitionTo(smartorder.StatusCompleted)
	_ = repo.UpdateRunStatus(ctx, run.ID, run.Status, run.CurrentStep, "")
	fmt.Printf("\nrun %s complete — open /customer/smart-order/%s/results\n", run.RunNumber, run.PublicID)
	return nil
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) * 100 / float64(total)
}

// smokeEnhancer wires the AI stage when the Gateway is configured, and returns
// nil when it is not — which the pipeline treats as "skip the stage" rather than
// as an error, exactly as the server and worker do.
func smokeEnhancer(gw gateway.Client, orgSvc *org.Service, log *slog.Logger) pipeline.Enhancer {
	if gw == nil || !gw.Enabled() {
		return nil
	}
	caps := aicapabilities.NewService(gw, log)
	caps.SetKeyResolver(func(ctx context.Context, orgID int64) (string, error) {
		o, err := orgSvc.GetOrganization(ctx, orgID)
		if err != nil || o == nil {
			return "", err
		}
		return o.AIVirtualKey, nil
	})
	return pipeline.NewGatewayEnhancer(&cliEnhanceAdapter{caps: caps})
}

// cliEnhanceAdapter bridges aicapabilities to the pipeline's gateway contract.
type cliEnhanceAdapter struct {
	caps *aicapabilities.Service
}

func (b *cliEnhanceAdapter) EnhanceBatch(ctx context.Context, batch pipeline.GatewayBatch) ([]pipeline.GatewayOutcome, error) {
	req := aicapabilities.EnhanceRequest{
		Catalog: make([]aicapabilities.CatalogEntry, 0, len(batch.Catalog)),
		Items:   make([]aicapabilities.EnhanceItem, 0, len(batch.Items)),
		// Attribution for the AI usage ledger: a smoke run is attributed like the real thing.
		Feature: matchflow.FeatureSmartOrder,
	}
	for _, c := range batch.Catalog {
		req.Catalog = append(req.Catalog, aicapabilities.CatalogEntry(c))
	}
	for _, it := range batch.Items {
		req.Items = append(req.Items, aicapabilities.EnhanceItem(it))
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

func coverageGateCLI(cs *workflow.CoverageService) pipeline.CoverageGate {
	return smartorder.CoverageFunc(func(ctx context.Context, vendorOrgID int64,
		day time.Weekday, lat, lng float64) (bool, int, error) {
		return cs.ServesPoint(ctx, vendorOrgID, day, workflow.Coord{Lat: lat, Lon: lng, CityID: nil})
	})
}
