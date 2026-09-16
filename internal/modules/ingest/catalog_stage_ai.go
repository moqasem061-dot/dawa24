package ingest

// The AI stage of the staging pass.
//
// catalog_stage.go reads the file and matches it. This file decides what is put
// to the model afterwards and what comes back: which settled rows are worth a
// second opinion, how the stage is driven, and how an answer — an acceptance or
// a disagreement — moves a row between the counters the vendor sees.

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// verifiable reports whether a settled match rests on a NAME, and is therefore
// worth a second opinion.
//
// A barcode is the same physical package and a supplier code the vendor mapped
// themselves is their own assertion about their own numbering; a model cannot
// improve on either, and asking spends the budget the ambiguous rows need.
func verifiable(level productmatch.MatchLevel) bool {
	switch level {
	case productmatch.MatchExact, productmatch.MatchStrong:
		return true
	}
	return false
}

// enhance runs the AI stage over the whole file and folds what it settled
// back onto the staged rows.
//
// Every failure here is silent by design: the vendor keeps a complete,
// deterministically matched staging table, which is a usable answer, and the
// review screen says what the stage managed rather than pretending it ran.
func (r *stagingRun) enhance(ctx context.Context) {
	s := r.svc
	if !r.session.Settings.UseAI || s.enhancer == nil {
		return
	}
	if len(r.open) == 0 {
		// Nothing to ask about at all — an empty file, or one whose every row
		// the reader rejected. The stage still records that it was on, because
		// "there was nothing to ask" and "the smart matching never ran" are
		// different things to tell a vendor.
		r.session.AI = AIStats{Ran: true}
		return
	}

	r.progress(ctx, 45, i18n.TDefault("w4_mod.s_389_389"))

	enh := NewEnhancement(s.enhancer, s.memory, r.index, s.log)
	enh.Stats.CeilingHit = r.ceilingHit
	enh.OnProgress = func(done, total int) {
		if total <= 0 {
			return
		}
		r.progress(ctx, 50+(done*45)/total,
			fmt.Sprintf(i18n.TDefault("w4_mod.d_d_390"), done, total))
	}

	// Retrieval is deliberately outside the streaming loop and outside the
	// request budget: it is index arithmetic that costs nothing but CPU, and
	// running it once over de-duplicated questions is a fraction of what
	// running it per row would have been.
	askable := enh.Retrieve(r.open)

	matches := enh.Run(ctx, askable)
	r.session.AI = enh.Stats

	if len(matches) > 0 {
		if err := s.imports.ApplyAIMatches(ctx, r.session.ID, matches); err != nil {
			// The rows keep their deterministic outcome and the counters are
			// left alone, so the screen and the table still agree.
			s.log.WarnContext(ctx, "AI matches not written to staging rows",
				"import", r.session.PublicID, "matches", len(matches), "error", err)
			return
		}
		r.recount(matches)
	}

	s.log.InfoContext(ctx, "vendor import AI enhancement finished",
		"import", r.session.PublicID, "questions", len(r.open),
		"reviewed", enh.Stats.Reviewed, "cache_hits", enh.Stats.CacheHits,
		"requests", enh.Stats.Requests, "improved", enh.Stats.Improved,
		"abstained", enh.Stats.Abstained, "rejected", enh.Stats.Rejected,
		"verified", enh.Stats.Verified, "disputed", enh.Stats.Disputed,
		"ceiling_hit", enh.Stats.CeilingHit)
}

// recount moves every AI-settled row out of the counter it was tallied under.
//
// It decrements the row's own original bucket rather than assuming "needs
// review": a row with candidates but a weak best score was counted as
// unmatched, and decrementing the wrong counter makes the review screen
// disagree with its own tabs.
func (r *stagingRun) recount(matches []AIMatch) {
	for _, m := range matches {
		bucket, ok := r.bucketOf[m.SourceRow]
		if !ok {
			continue
		}
		want := bucketMatched
		if m.Level == aiLevelDisputed {
			// The engine settled this row and the model would not confirm it.
			// It moves the other way — out of the matched count and into the
			// review count — because the vendor's screen must show it, and a
			// counter that still called it matched would be the one place the
			// disagreement was invisible.
			want = bucketReview
		}
		if bucket == want {
			continue
		}
		r.count(bucket, -1)
		r.count(want, 1)
		r.bucketOf[m.SourceRow] = want
	}
}

// progress records how far staging has reached, for the waiting screen.
//
// It carries the caller's context because the write is tenant-scoped: a
// progress row written outside the vendor's tenancy is a row the policy refuses
// and a bar that never moves.
func (r *stagingRun) progress(ctx context.Context, percent int, note string) {
	if err := r.svc.imports.Progress(ctx, r.session.ID, percent, note); err != nil {
		r.svc.log.WarnContext(ctx, "import progress not recorded",
			"import", r.session.PublicID, "error", err)
	}
	r.session.ProgressPercent = percent
	r.session.ProgressNote = note
}

// count adjusts one of the match counters by delta.
func (r *stagingRun) count(b matchBucket, delta int) {
	switch b {
	case bucketMatched:
		r.counts.matched += delta
	case bucketReview:
		r.counts.review += delta
	default:
		r.counts.unmatched += delta
	}
}
