package pipeline

// Applying an answer, and refusing one.
//
// This is the file that decides what the model is allowed to change. Its bias is
// stated once and applies throughout: a wrong confident match ships the wrong
// medicine to a patient and gives nobody a signal that anything went wrong,
// while a refused match merely leaves a line for a human to look at. So every
// guard here fails toward the deterministic outcome.
//
// The guards themselves live in matchflow.Verdict, because all four importers
// need the same ones and the four copies had drifted. What lives here is what
// each verdict MEANS to a purchase order:
//
//   - keep: the deterministic result stands.
//   - apply: the model resolved a line the engine could not.
//   - review: the two methods disagree about a line the engine had already
//     applied, and the buyer is asked. This is the outcome the verification
//     pass exists to produce, and it is the only one that can take a line OFF
//     the automatic path — which is why it lowers the confidence rather than
//     clearing the match: the buyer needs to see what was proposed in order to
//     judge it.

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// confDisputed is the confidence a line carries once the deterministic engine
// and the model have disagreed about it.
//
// Below Cutoff, so the line leaves the automatic path and reaches the review
// screen. Above the floor at which a line is reported as unmatched, because it
// is not unmatched: two methods each named a product and they named different
// ones, and the buyer needs both in front of them.
const confDisputed = 0.60

// apply validates a batch's answers and writes the ones that survive.
func (e *Enhancement) apply(req matchflow.Request, groups map[string][]Review,
	outcomes []matchflow.Decision) []smartorder.CachedDecision {

	saved := make([]smartorder.CachedDecision, 0, len(outcomes))

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, out := range outcomes {
		key, ok := req.Keys[out.Ref]
		if !ok {
			e.Stats.Rejected++
			continue
		}
		group := groups[key]
		if len(group) == 0 {
			e.Stats.Rejected++
			continue
		}
		lead := group[0]

		j := e.judge(req, lead, out)
		verdict := matchflow.Verdict(j, out.ProductID)
		e.record(verdict, group, lead, out)

		if matchflow.Remember(j, out.ProductID) {
			saved = append(saved, smartorder.CachedDecision{
				Key:             key,
				NormName:        lead.Line.NormName,
				ChosenProductID: out.ProductID,
				Confidence:      out.Confidence,
				Reason:          out.Reason,
				PromptVersion:   PromptVersion,
			})
		}
	}
	return saved
}

// judge gathers what the shared rules need to decide one answer.
func (e *Enhancement) judge(req matchflow.Request, r Review,
	out matchflow.Decision) matchflow.Judgement {

	j := matchflow.Judgement{
		Settled:    r.Settled,
		Current:    r.Line.MatchedProductID,
		Confidence: out.Confidence,
		Floor:      ceilings.MinApplyConfidence,
	}
	if out.ProductID == nil || *out.ProductID <= 0 {
		return j
	}
	_, j.Offered = req.Offered[*out.ProductID]
	j.Conflicts = !e.index.IdentityConflict(r.Row, *out.ProductID).None()
	return j
}

// record writes one verdict across every line that asked the question.
func (e *Enhancement) record(verdict matchflow.Outcome, group []Review, lead Review,
	out matchflow.Decision) {

	switch verdict {
	case matchflow.OutcomeApply:
		for _, r := range group {
			before := r.Line.MatchedProductID
			forceMatch(r.Line, *out.ProductID, smartorder.MethodAI, out.Confidence)
			if before == nil || *before != *out.ProductID {
				e.Stats.Improved++
			} else {
				e.Stats.Confirmed++
			}
		}

	case matchflow.OutcomeReview:
		for _, r := range group {
			r.Line.MatchConfidence = confDisputed
			r.Line.OutcomeReason = disputeReason(out)
		}
		e.Stats.Disputed += len(group)
		if e.log != nil {
			e.log.Debug("AI review disagreed with an applied match",
				"line_id", lead.Line.ID, "text", lead.Line.RawName,
				"engine_product", lead.Line.MatchedProductID,
				"model_product", out.ProductID, "confidence", out.Confidence)
		}

	default:
		// Nothing changes. Which of the several reasons it was is worth
		// counting, because "the model confirmed it" and "the model was
		// overruled by the identity guard" are different facts about a run.
		switch {
		case out.ProductID == nil || *out.ProductID <= 0:
			e.Stats.Abstained += len(group)
		case out.Confidence < ceilings.MinApplyConfidence:
			e.Stats.Abstained += len(group)
		case sameProduct(lead.Line.MatchedProductID, out.ProductID):
			e.Stats.Confirmed += len(group)
		default:
			e.refuse(lead, *out.ProductID, len(group))
		}
	}
}

// disputeReason is what the buyer is told about a line two methods disagreed on.
func disputeReason(out matchflow.Decision) string {
	if out.ProductID == nil || *out.ProductID <= 0 {
		return "المراجعة الذكية لم تؤكد الصنف الذي اختاره المطابق الآلي؛ يلزم التأكيد يدوياً."
	}
	return "المراجعة الذكية رشّحت صنفاً مختلفاً عن اختيار المطابق الآلي؛ يلزم التأكيد يدوياً."
}

func sameProduct(a, b *int64) bool { return a != nil && b != nil && *a == *b }

// refuse records a rejected decision and says why.
//
// The reason is logged rather than shown: a buyer does not need to know that the
// model proposed something and was overruled, but an operator asking "is the AI
// stage helping or fighting the matcher?" needs the answer, and it is not
// recoverable after the fact.
//
// It touches Stats directly rather than through count(), because every caller
// already holds e.mu.
func (e *Enhancement) refuse(r Review, productID int64, lines int) {
	c := e.index.IdentityConflict(r.Row, productID)
	e.Stats.Rejected += lines
	e.Stats.RefusedBy[c.Kind]++
	if e.log != nil {
		e.log.Debug("AI match refused by the identity guard",
			"line_id", r.Line.ID, "text", r.Line.RawName,
			"product_id", productID, "conflict", c.Kind, "detail", c.Detail)
	}
}

// forceMatch records an AI resolution.
//
// Unlike setMatch it does not refuse a lower confidence, because that is exactly
// the case this stage exists for: a line the scorer guessed at 0.42 being
// replaced by a considered answer at 0.88 — or by a *different* product at 0.75,
// which is still better than a guess the engine itself would not stand behind.
func forceMatch(l *smartorder.Line, productID int64, method smartorder.MatchMethod, confidence float64) {
	l.MatchedProductID = &productID
	l.MatchMethod = method
	l.MatchConfidence = confidence
}

// flush persists the decisions and the aliases they imply.
func (e *Enhancement) flush(ctx context.Context, decisions []smartorder.CachedDecision) {
	if len(decisions) == 0 {
		return
	}
	e.persistMu.Lock()
	defer e.persistMu.Unlock()
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	// A cache write failing must not fail the run: the decisions were still
	// applied, they will simply be paid for again next time.
	_ = e.repo.SaveDecisions(persistCtx, decisions)

	// Each accepted match is also recorded as an *untrusted* alias, source
	// 'ai_confirmed', which the deterministic alias tier deliberately excludes.
	// The row exists so a buyer accepting the match can promote it and so an
	// operator can see what the model has been deciding — not so the next
	// import trusts it. One confident mistake propagating silently to every
	// pharmacy is precisely what this guards against.
	for _, d := range decisions {
		if d.ChosenProductID == nil || d.NormName == "" {
			continue
		}
		_ = e.repo.SaveAlias(persistCtx, *d.ChosenProductID, d.NormName, "ai_confirmed", d.Confidence)
	}
}

// applyCache resolves what is already known and returns the questions still
// worth asking.
//
// A remembered answer is re-checked against the catalogue's own record before it
// is applied, exactly as a fresh one is: the catalogue moves between imports, and
// a decision that was sound in March can conflict in June.
func (e *Enhancement) applyCache(ctx context.Context, groups map[string][]Review) map[string][]Review {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	cached, err := e.repo.LookupDecisions(ctx, keys)
	if err != nil {
		cached = nil // a cache miss is never fatal
	}

	pending := make(map[string][]Review, len(groups))
	for key, group := range groups {
		d, known := cached[key]
		if !known {
			pending[key] = group
			continue
		}
		e.Stats.CacheHits += len(group)
		lead := group[0]
		j := matchflow.Judgement{
			Settled:    lead.Settled,
			Current:    lead.Line.MatchedProductID,
			Offered:    true, // it was offered when the answer was bought
			Confidence: d.Confidence,
			Floor:      ceilings.MinApplyConfidence,
		}
		if d.ChosenProductID != nil && *d.ChosenProductID > 0 {
			j.Conflicts = !e.index.IdentityConflict(lead.Row, *d.ChosenProductID).None()
		}
		e.record(matchflow.Verdict(j, d.ChosenProductID), group, lead,
			matchflow.Decision{ProductID: d.ChosenProductID, Confidence: d.Confidence, Reason: d.Reason})
	}
	e.Stats.Reviewed = len(pending)
	return pending
}
