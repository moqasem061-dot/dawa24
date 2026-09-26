package ui

import (
	"context"
	"log/slog"
	"sync"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// The saving list's AI stage.
//
// It runs exactly the stage the other three importers run: the same prompt
// version, the same ceilings, the same shared decision cache, the same shared
// catalogue window, the same planner, the same identity re-check before an
// answer is written. Nothing here is a second implementation of the idea — the
// parts that could drift live in internal/shared/matchflow, and this file is
// the plumbing between them and a list of staged rows.
//
// Two things changed when the planner became shared, and both were defects
// only this importer had.
//
// It built ONE catalogue window for the whole file and then sent that entire
// window with every request. A short list never noticed. A long one paid for
// its whole catalogue window thirty times over, and past a few thousand rows
// the request stopped being answerable at all — the byte budget the other
// importers enforce was not enforced here, so nothing said so.
//
// And it asked only about rows the deterministic tiers left unlinked, which
// means a pharmacy's own list was checked everywhere except where the engine
// was confident. Every row whose link rests on a name now goes to the model:
// the unlinked ones to be resolved, the linked ones to be confirmed.

// enhanceSaving is the single entry point every staging path calls.
//
// The toggle is applied here rather than at each call site. There are four
// staging paths in this package, and the first version of this feature was
// wired into two of them — so it was present in the flow nobody uses and absent
// from the one the upload screen actually drives. One function that every path
// must go through is how that stops being possible.
// SavingAIProgressFunc reports progress during the AI matching phase.
type SavingAIProgressFunc func(doneBatches, totalBatches, improved int)

func (h *UIHandler) enhanceSaving(
	ctx context.Context, useAI bool, engine *SavingProductMatchEngine, items []*StagedSavingItem,
) int {
	return h.enhanceSavingWithProgress(ctx, useAI, engine, items, nil)
}

// enhanceSavingWithProgress runs the AI stage reporting progress as batches complete.
func (h *UIHandler) enhanceSavingWithProgress(
	ctx context.Context, useAI bool, engine *SavingProductMatchEngine, items []*StagedSavingItem,
	onProgress SavingAIProgressFunc,
) int {
	if !useAI {
		return 0
	}
	return enhanceSavingItemsWithProgress(ctx, h.matchEnhancer, h.matchMemory, engine, items, h.log, onProgress)
}

// savingAIUnavailableReason explains a disabled switch.
//
// A toggle that ticks and then does nothing is worse than one that says why it
// cannot: the old strategy dropdown promised smart matching from an engine that
// had none, and nobody could tell the promise was empty.
func savingAIUnavailableReason(e matchflow.Enhancer, lang ...string) string {
	if e != nil {
		return ""
	}
	l := "ar"
	if len(lang) > 0 && lang[0] != "" {
		l = lang[0]
	}
	return i18n.T(l, "saving.ai.unavailable_reason")
}

// savingAICeilings is what one saving-list run may spend.
// Uses ProfileSaving with high concurrency (up to 6 workers) and adaptive dynamic batching.
var savingAICeilings = matchflow.For(matchflow.ProfileSaving)

// enhanceSavingItems asks the model about the rows the deterministic tiers left
// unlinked, checks the ones they linked, and writes back what survives.
//
// It never returns an error and never fails the import. Every failure path —
// no Gateway, no answer, a malformed response, an answer that does not survive
// the re-check — leaves the row exactly as the deterministic engine left it,
// which is a usable result the user can correct by hand.
//
// The returned count is rows whose link CHANGED, which is what the screen
// reports. A confirmation changes nothing and is not counted as an improvement,
// because it is not one.
func enhanceSavingItems(
	ctx context.Context,
	enhancer matchflow.Enhancer,
	memory matchflow.Memory,
	engine *SavingProductMatchEngine,
	items []*StagedSavingItem,
	log *slog.Logger,
) int {
	return enhanceSavingItemsWithProgress(ctx, enhancer, memory, engine, items, log, nil)
}

func enhanceSavingItemsWithProgress(
	ctx context.Context,
	enhancer matchflow.Enhancer,
	memory matchflow.Memory,
	engine *SavingProductMatchEngine,
	items []*StagedSavingItem,
	log *slog.Logger,
	onProgress SavingAIProgressFunc,
) (improved int) {
	if enhancer == nil || engine == nil || engine.index == nil || len(items) == 0 {
		return 0
	}

	asked := planSavingQuestions(engine.index, items)
	if len(asked) == 0 {
		return 0
	}

	// The cache answers first, and this is the tier that was missing: a
	// pharmacy re-uploads its saving list every few weeks with a handful of
	// rows changed, and every one of those uploads used to be paid for in full
	// — asking the same model, through the same prompt, the questions the
	// vendor import and the smart order had already bought answers to.
	improved += applySavingMemory(ctx, memory, engine, asked)

	pending := make([]matchflow.Question, 0, len(asked))
	for _, q := range asked {
		if !q.answered {
			pending = append(pending, q.question)
		}
	}
	if len(pending) == 0 {
		return improved
	}

	var orgID, userID int64
	if actor, ok := authctx.From(ctx); ok {
		orgID = actor.OrgID
		if orgID <= 0 {
			orgID = actor.OrganizationID
		}
		userID = actor.UserID
	}
	if orgID <= 0 {
		if tid, ok := database.TenantFrom(ctx); ok && tid > 0 {
			orgID = tid
		}
	}

	total := len(pending)
	if log != nil {
		log.InfoContext(ctx, "starting saving AI matching pass",
			"items", len(items), "asked", len(asked), "pending", total)
	}
	if onProgress != nil {
		onProgress(0, total, improved)
	}

	var (
		mu         sync.Mutex
		remembered []matchflow.Remembered
		done       int
	)
	stats := matchflow.Run(ctx, enhancer, matchflow.Spec{
		Profile:        matchflow.ProfileSaving,
		Rows:           len(items),
		Questions:      pending,
		Feature:        matchflow.FeatureSavingsImport,
		OrganizationID: orgID,
		UserID:         userID,
		Screen: func(key string, productID int64) bool {
			mu.Lock()
			q, ok := asked[key]
			mu.Unlock()
			return ok && engine.index.IdentityConflict(q.row, productID).None()
		},
		Describe: func(id int64) (matchflow.CatalogEntry, bool) {
			p, ok := engine.index.Lookup(id)
			if !ok {
				return matchflow.CatalogEntry{}, false
			}
			return matchflow.CatalogEntry{
				ProductID: p.ID, NameAR: p.NameAR, NameEN: p.NameEN,
				Scientific: p.Scientific, DosageForm: p.DosageForm,
				Concentration: p.Concentration, Manufacturer: p.Manufacturer,
			}, true
		},
		OnAnswer: func(a matchflow.Answer) {
			mu.Lock()
			defer mu.Unlock()
			q, ok := asked[a.Key]
			if !ok {
				return
			}
			d := a.Decision
			j := savingJudgement(engine, q, d.Confidence, d.ProductID)
			// The engine checked the answer against the request's shared window,
			// which is wider than this row's own shortlist.
			if d.ProductID != nil && a.Offered && engine.known[*d.ProductID] {
				j.Offered = true
			}
			j.Verified = a.Verified()
			improved += applySavingVerdict(engine, q, matchflow.Verdict(j, d.ProductID), d)
			if matchflow.Remember(j, d.ProductID) && q.firstTarget() != nil {
				remembered = append(remembered, matchflow.Remembered{
					Key:             a.Key,
					NormName:        productmatch.NormalizeText(q.firstTarget().NameProduct),
					ChosenProductID: d.ProductID,
					Confidence:      d.Confidence,
					Reason:          d.Reason,
					PromptVersion:   matchflow.PromptVersion,
				})
			}
		},
		OnDone: func(keys []string) {
			if onProgress == nil {
				return
			}
			mu.Lock()
			done += len(keys)
			n, imp := done, improved
			mu.Unlock()
			onProgress(n, total, imp)
		},
	})
	if log != nil {
		log.InfoContext(ctx, "saving AI matching pass finished",
			"improved", improved, "requests", stats.Requests, "checks", stats.CheckRequests,
			"check_accepted", stats.CheckAccepted, "check_rejected", stats.CheckRejected,
			"failed", stats.Failed, "ceiling_hit", stats.CeilingHit)
	}

	saveSavingMemory(ctx, memory, remembered)
	return improved
}

// savingJudgement gathers what the shared rules need to decide one answer.
func savingJudgement(engine *SavingProductMatchEngine, q *savingQuestion,
	confidence float64, proposed *int64) matchflow.Judgement {

	j := matchflow.Judgement{
		Settled:    q.settled,
		Current:    q.current,
		Confidence: confidence,
		Floor:      savingAICeilings.MinApplyConfidence,
	}
	if proposed == nil || *proposed <= 0 {
		return j
	}
	j.Offered = engine.known[*proposed] && q.offers(*proposed)
	j.Conflicts = !engine.index.IdentityConflict(q.row, *proposed).None()
	return j
}

// applySavingVerdict writes one verdict onto the staged rows, and reports the number
// of row targets whose link changed.
func applySavingVerdict(engine *SavingProductMatchEngine, q *savingQuestion,
	verdict matchflow.Outcome, d matchflow.Decision) int {

	changedCount := 0
	switch verdict {
	case matchflow.OutcomeApply:
		id := *d.ProductID
		mName, mSKU := engine.Describe(id)
		for _, target := range q.targets {
			target.ProductID = &id
			target.MatchType = savingMatchTypeAI
			target.Confidence = d.Confidence
			target.MasterProductName, target.MasterProductSKU = mName, mSKU
			changedCount++
		}

	case matchflow.OutcomeReview:
		// The engine linked this row and the model would not confirm it. The
		// link is withdrawn rather than replaced: a pharmacy's own list is
		// theirs to correct, and an unlinked row on the review screen is a
		// question they can answer, where a link neither method stands behind
		// is one they would never think to check.
		for _, target := range q.targets {
			target.ProductID = nil
			target.MatchType = savingMatchTypeDisputed
			target.Confidence = 0
			target.MasterProductName, target.MasterProductSKU = "", ""
			changedCount++
		}

	default:
		// A confirmation relabels a fuzzy match as AI-backed — but only when
		// the model named THE SAME product. A keep verdict also covers the
		// model naming a different product that the identity guard refused,
		// and relabelling then showed a match the AI disagreed with as one it
		// had endorsed.
		if d.ProductID != nil && q.current != nil && *d.ProductID == *q.current &&
			d.Confidence >= savingAICeilings.MinApplyConfidence {
			for _, target := range q.targets {
				if target.MatchType == "fuzzy_name" {
					target.MatchType = savingMatchTypeAI
					target.Confidence = d.Confidence
					changedCount++
				}
			}
		}
	}
	return changedCount
}

// The match types this stage writes, in the vocabulary the review screen and
// the stored sessions already use.
const (
	savingMatchTypeAI       = "ai"
	savingMatchTypeDisputed = "ai_disputed"
)

// applySavingMemory resolves what the shared cache already knows, marking the
// questions it answered.
//
// A remembered answer is re-checked against the catalogue's own record before
// it is applied, exactly as a fresh one is: the catalogue moves between
// imports, and a decision that was sound in March can conflict in June.
func applySavingMemory(ctx context.Context, memory matchflow.Memory,
	engine *SavingProductMatchEngine, asked map[string]*savingQuestion) (improved int) {

	if memory == nil || len(asked) == 0 {
		return 0
	}
	keys := make([]string, 0, len(asked))
	for k := range asked {
		keys = append(keys, k)
	}
	known := matchflow.Recall(ctx, memory, keys)

	for key, q := range asked {
		d, ok := known[key]
		if !ok {
			continue
		}
		q.answered = true
		j := matchflow.Judgement{
			Settled:    q.settled,
			Current:    q.current,
			Offered:    true, // it was offered when the answer was bought
			Confidence: d.Confidence,
			Floor:      savingAICeilings.MinApplyConfidence,
		}
		if d.ChosenProductID != nil && *d.ChosenProductID > 0 {
			j.Offered = engine.known[*d.ChosenProductID]
			j.Conflicts = !engine.index.IdentityConflict(q.row, *d.ChosenProductID).None()
		}
		// Answers under this prompt version are remembered only after the
		// pair check.
		j.Verified = d.ChosenProductID != nil
		improved += applySavingVerdict(engine, q, matchflow.Verdict(j, d.ChosenProductID),
			matchflow.Decision{ProductID: d.ChosenProductID, Confidence: d.Confidence})
	}
	return improved
}

// saveSavingMemory writes the run's decisions and the aliases they imply,
// without letting either failure fail the import.
func saveSavingMemory(ctx context.Context, memory matchflow.Memory, decisions []matchflow.Remembered) {
	if memory == nil || len(decisions) == 0 {
		return
	}
	_ = memory.Save(ctx, decisions)
	for _, d := range decisions {
		if d.ChosenProductID == nil || d.NormName == "" {
			continue
		}
		_ = memory.SaveAlias(ctx, *d.ChosenProductID, d.NormName, "ai_confirmed", d.Confidence)
	}
}
