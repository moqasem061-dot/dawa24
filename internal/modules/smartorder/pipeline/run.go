package pipeline

import (
	"context"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Runner drives a smart order from staged rows to selected suppliers.
type Runner struct {
	repo          smartorder.Repository
	coverage      CoverageGate
	institutional InstitutionalGate
	gate          smartorder.AvailabilityGate
	ai            Enhancer
	log           *slog.Logger
}

// NewRunner constructs the pipeline.
func NewRunner(repo smartorder.Repository, cov CoverageGate, inst InstitutionalGate,
	ai Enhancer, log *slog.Logger) *Runner {
	return &Runner{repo: repo, coverage: cov, institutional: inst, ai: ai, log: log}
}

// SetAvailabilityGate wires the unified purchase availability gate.
func (r *Runner) SetAvailabilityGate(gate smartorder.AvailabilityGate) {
	r.gate = gate
}

// Execute runs every stage for one smart order.
//
// The stage order is the performance design: exact tiers settle the bulk over
// the whole file, the in-memory scorer handles what is left, and AI sees only
// the residue of the residue. Progress is written after each stage so a buyer
// watching the page sees movement, and a buyer who left can tell where it got to.
func (r *Runner) Execute(ctx context.Context, run *smartorder.Run, cfg *smartorder.Config, branch BranchLocation) error {
	started := time.Now()

	lines, _, err := r.repo.ListLines(ctx, run.ID, smartorder.LineFilter{All: true})
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		return nil
	}
	run.Stats.TotalRows = len(lines)

	// Every stage reports when it FINISHES, with what it actually settled.
	//
	// It used to report on entry, always with processed=0, so the screen showed
	// "0 / 804" under four stage names for the whole run and the buyer learned
	// nothing from any of it. A count is only worth showing once it counts
	// something.

	// Stage 1 — normalise. Pure CPU, no I/O.
	Normalize(lines)
	smartorder.ApplyQuantities(cfg, lines)
	r.emit(ctx, run, smartorder.StageNormalize, len(lines), len(lines),
		i18n.TDefault("w4_mod.s_451_451"), "Preparing items")

	// Stage 2 — the exact tiers, each one query for the whole file.
	resolver := NewResolver(r.repo, cfg)
	if err := resolver.Resolve(ctx, lines); err != nil {
		return err
	}
	residual := Unresolved(lines)
	r.emit(ctx, run, smartorder.StageResolve, len(lines)-len(residual), len(lines),
		i18n.TDefault("w4_mod.s_428_428"), "Matching codes and names")

	// Stage 3 — score what is left against an in-memory catalogue.
	matcher := NewMatcher(r.repo, cfg.MinMatchScore)
	if err := matcher.Load(ctx); err != nil {
		return err
	}
	if matcher.Size() == 0 {
		// An empty catalogue is a broken run, not a file full of unknown
		// products. Saying so prevents a support ticket that starts "nothing
		// matched".
		r.log.ErrorContext(ctx, "catalogue index is empty; every line will be unmatched",
			"run_id", run.ID)
	}
	reviews := matcher.Score(residual)
	r.emit(ctx, run, smartorder.StageCandidates, len(lines)-len(reviews), len(lines),
		i18n.TDefault("w4_mod.s_429_429"), "Matching remaining items")

	// The deterministic engine is done. Saying so out loud, as its own event, is
	// what lets the buyer see that ordinary matching finished and something else
	// is still working — without it, a run three quarters through the AI stage
	// is indistinguishable from one that has hung.
	deterministicMS := int(time.Since(started).Milliseconds())
	run.DeterministicMS = &deterministicMS
	r.emit(ctx, run, smartorder.StageInitialDone, len(lines)-len(reviews), len(lines),
		i18n.TDefault("w4_mod.s_430_430"), "Initial matching completed")

	// Stage 4 — AI enhancement, on exactly the lines the engine left as
	// غير مطابق or مطلوب للمراجعة, and on nothing else.
	//
	// The results are NOT released to the buyer while this runs: the run stays
	// in `processing` until the whole pipeline finishes, so what they finally
	// see already includes whatever this improved.
	r.enhance(ctx, run, cfg, matcher, lines, reviews)

	if err := r.repo.UpdateLines(ctx, lines); err != nil {
		return err
	}

	// Stage 5 — suppliers, coverage, Corporate Operations, selection.
	r.emit(ctx, run, smartorder.StageSelect, 0, len(lines), i18n.TDefault("w4_mod.s_432_432"), "Finding suppliers")
	supplier := NewSupplier(r.repo, r.coverage, r.institutional, cfg, branch)
	if r.gate != nil {
		supplier.SetAvailabilityGate(r.gate)
	}
	total, err := supplier.Resolve(ctx, lines)
	if err != nil {
		return err
	}

	if err := r.repo.UpdateLines(ctx, lines); err != nil {
		return err
	}
	r.emit(ctx, run, smartorder.StageSelect, len(lines), len(lines),
		i18n.TDefault("w4_mod.s_432_432"), "Finding suppliers")

	run.Stats = smartorder.CountByOutcome(lines)
	run.EstimatedTotal = total
	exceeded, overage, hasBudget := cfg.BudgetStatus(total)
	if hasBudget {
		run.BudgetExceeded = &exceeded
		if exceeded {
			run.BudgetOverage = &overage
		}
	}
	totalMS := int(time.Since(started).Milliseconds())
	run.TotalMS = &totalMS

	// A terminal event, so the bar reaches 100 rather than stopping at 99 and
	// being replaced by the results page mid-animation.
	r.emit(ctx, run, smartorder.StageFinalize, 1, 1,
		i18n.TDefault("w4_mod.s_433_433"), "Matching completed")

	return r.repo.UpdateRunStats(ctx, run)
}

// enhance runs the AI stage over the lines the deterministic engine could not
// settle, and folds what it learns back into those same lines.
//
// It is a no-op — not a failure — when the buyer turned AI off, when the Gateway
// is unconfigured, or when the engine settled everything. Nothing it does can
// fail the run: the deterministic outcome is always there to fall back to, which
// is what lets a pharmacy order when the Gateway is down.
func (r *Runner) enhance(ctx context.Context, run *smartorder.Run, cfg *smartorder.Config,
	matcher *Matcher, lines []*smartorder.Line, reviews []Review) {
	if !cfg.UseAIMatching || r.ai == nil || len(lines) == 0 {
		return
	}

	// Retrieval runs here rather than inside the fuzzy stage so that a run which
	// never reaches AI never pays for it. It also answers which of these lines
	// the catalogue could plausibly settle at all; the rest keep the honest
	// deterministic outcome and are never sent.
	askable := matcher.Retrieve(reviews)

	total := len(askable)
	if total == 0 {
		return
	}
	enh := NewEnhancement(r.repo, r.ai, matcher.Index(), r.log)
	// This is the stage that waits on a network, so it is the one that has to
	// move the bar while it works. Reporting only on entry and exit left the
	// buyer watching a still number through the slowest minute of the run.
	enh.OnProgress = func(done int) {
		r.emit(ctx, run, smartorder.StageAIEnhance, done, total,
			i18n.TDefault("w4_mod.s_431_431"), "AI is improving uncertain matches")
	}
	enh.Run(ctx, askable)

	run.AI.Calls = enh.Stats.Requests
	run.AI.LinesReviewed = enh.Stats.Reviewed
	run.AI.LinesAdjudicated = enh.Stats.Improved + enh.Stats.Confirmed + enh.Stats.Abstained
	run.AI.LinesImproved = enh.Stats.Improved
	run.AI.CacheHits = enh.Stats.CacheHits
	run.AI.CeilingHit = enh.Stats.CeilingHit
	run.AI.LinesVerified = enh.Stats.Verified
	run.AI.LinesDisputed = enh.Stats.Disputed

	r.log.InfoContext(ctx, "smart order AI enhancement finished",
		"run_id", run.ID, "reviewed", enh.Stats.Reviewed, "cache_hits", enh.Stats.CacheHits,
		"requests", enh.Stats.Requests, "improved", enh.Stats.Improved,
		"confirmed", enh.Stats.Confirmed, "abstained", enh.Stats.Abstained,
		"rejected", enh.Stats.Rejected, "refused_by", enh.Stats.RefusedBy,
		"verified", enh.Stats.Verified, "disputed", enh.Stats.Disputed,
		"ceiling_hit", enh.Stats.CeilingHit)

	r.emit(ctx, run, smartorder.StageAIEnhance, total, total,
		i18n.TDefault("w4_mod.s_452_452"), "AI enhancement completed")

	if enh.Stats.CeilingHit {
		r.emitWarn(ctx, run, smartorder.StageAIEnhance,
			i18n.TDefault("w4_mod.s_453_453"),
			"AI enhancement limit reached; some items were left as matched deterministically")
	}
}

func (r *Runner) emit(ctx context.Context, run *smartorder.Run, stage smartorder.Stage,
	processed, total int, ar, en string) {
	p, t := processed, total
	_ = r.repo.AppendEvent(ctx, &smartorder.Event{
		RunID:          run.ID,
		OrganizationID: run.OrganizationID,
		Stage:          stage,
		Processed:      &p,
		Total:          &t,
		Message:        i18n.Text{"ar": ar, "en": en},
	})
}

func (r *Runner) emitWarn(ctx context.Context, run *smartorder.Run, stage smartorder.Stage, ar, en string) {
	_ = r.repo.AppendEvent(ctx, &smartorder.Event{
		RunID:          run.ID,
		OrganizationID: run.OrganizationID,
		Stage:          stage,
		Message:        i18n.Text{"ar": ar, "en": en},
		Level:          "warn",
	})
}
