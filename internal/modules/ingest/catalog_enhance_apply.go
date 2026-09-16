package ingest

// Applying an answer, and refusing one.
//
// This is the file that decides what the model is allowed to change. Its bias
// is stated once and applies throughout: a wrong confident match ties a
// vendor's price and stock to the wrong medicine — a pharmacy then orders it
// believing the catalogue — while a refused match merely leaves a row on the
// review screen for a person to look at. So every guard here fails toward the
// deterministic outcome.
//
// The guards themselves live in matchflow.Verdict, because all four importers
// need the same ones and the four copies had drifted. What lives here is what
// each verdict MEANS to a vendor's catalogue:
//
//   - keep: the deterministic result stands.
//   - apply: the model resolved a row the engine could not.
//   - review: the model would not confirm a match the engine had already
//     applied. The row keeps its product and leaves the settled bucket, so the
//     vendor is asked. This is the outcome the verification pass exists to
//     produce, and it is the only one that can take a row OFF the automatic
//     path.
//
// Nothing here writes to the database. The stage hands back accepted matches
// and the caller applies them to the staged rows in one statement, which is
// what keeps a twenty-five-thousand-row import to a fixed number of round trips.

import (
	"context"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// apply validates a request's answers and records the ones that survive.
func (e *Enhancement) apply(req matchflow.Request, asked map[string]*openRow,
	outcomes []matchflow.Decision) []CachedDecision {

	saved := make([]CachedDecision, 0, len(outcomes))

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, out := range outcomes {
		key, ok := req.Keys[out.Ref]
		if !ok {
			e.Stats.Rejected++
			continue
		}
		r, ok := asked[key]
		if !ok {
			e.Stats.Rejected++
			continue
		}

		j := e.judge(req, r, out.Confidence, out.ProductID)
		e.record(matchflow.Verdict(j, out.ProductID), r, out.Confidence, out.ProductID, out.Reason)

		if matchflow.Remember(j, out.ProductID) {
			saved = append(saved, CachedDecision{
				Key:             key,
				NormName:        r.normName,
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
func (e *Enhancement) judge(req matchflow.Request, r *openRow,
	confidence float64, proposed *int64) matchflow.Judgement {

	j := matchflow.Judgement{
		Settled:    r.settled,
		Current:    r.guess,
		Confidence: confidence,
		Floor:      ceilings.MinApplyConfidence,
	}
	if proposed == nil || *proposed <= 0 {
		return j
	}
	_, j.Offered = req.Offered[*proposed]
	j.Conflicts = !e.index.IdentityConflict(r.row, *proposed).None()
	return j
}

// record writes one verdict onto the row that asked.
func (e *Enhancement) record(verdict matchflow.Outcome, r *openRow,
	confidence float64, proposed *int64, reason string) {

	rows := len(r.sourceRows)
	switch verdict {
	case matchflow.OutcomeApply:
		e.resolve(r, *proposed, confidence, reason)

	case matchflow.OutcomeReview:
		if r.disputed {
			return
		}
		r.disputed = true
		e.Stats.Disputed += rows
		if e.log != nil {
			e.log.Debug("AI review disagreed with an applied match",
				"source_row", r.firstRow(), "text", r.row.DisplayName(),
				"engine_product", r.guess, "model_product", proposed,
				"confidence", confidence)
		}

	default:
		switch {
		case proposed == nil || *proposed <= 0, confidence < ceilings.MinApplyConfidence:
			e.Stats.Abstained += rows
		case sameProduct(r.guess, proposed):
			// A confirmation. Nothing changes, and that is the point: the row
			// was already right and now two methods say so.
		default:
			e.refuse(r, *proposed, rows)
		}
	}
}

func sameProduct(a, b *int64) bool { return a != nil && b != nil && *a == *b }

// resolve records an accepted answer on the row that asked.
//
// Unlike the deterministic path it does not refuse a lower score, because that
// is exactly the case this stage exists for: a row the scorer guessed at 0.42
// being settled by a considered answer at 0.88 — or by a *different* product at
// 0.85, which is still better than a guess the engine itself would not stand
// behind. Only rows the engine left unsettled ever reach here, so a confident
// deterministic result is never overwritten.
func (e *Enhancement) resolve(r *openRow, productID int64, confidence float64, reason string) {
	if r.answer != nil {
		return
	}
	r.answer = &aiAnswer{
		ProductID: productID,
		Score:     confidence,
		Reason:    aiReason(reason),
	}
	e.Stats.Improved += len(r.sourceRows)
}

func aiReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return i18n.TDefault("w4_mod.s_383_383")
	}
	return i18n.TDefault("ingest.ai_match_prefix") + reason
}

// refuse records a rejected decision and says why.
//
// The reason is logged rather than shown: a vendor does not need to know that
// the model proposed something and was overruled, but an operator asking "is
// the AI stage helping or fighting the matcher?" needs the answer, and it is
// not recoverable after the fact.
//
// It touches Stats directly rather than through count(), because every caller
// already holds e.mu.
func (e *Enhancement) refuse(r *openRow, productID int64, rows int) {
	c := e.index.IdentityConflict(r.row, productID)
	e.Stats.Rejected += rows
	if e.Stats.RefusedBy == nil {
		e.Stats.RefusedBy = map[string]int{}
	}
	e.Stats.RefusedBy[c.Kind]++
	if e.log != nil {
		e.log.Debug("AI match refused by the identity guard",
			"source_row", r.firstRow(), "text", r.row.DisplayName(),
			"product_id", productID, "conflict", c.Kind, "detail", c.Detail)
	}
}

// applyCache resolves what is already known and returns the questions still
// worth asking.
//
// This is the cheapest tier there is and the reason a vendor's weekly re-upload
// costs almost nothing: the cache is shared across every feature that asks this
// question, so an answer bought once is reused wherever the same product is
// written the same way against the same shortlist.
func (e *Enhancement) applyCache(ctx context.Context, rows []*openRow) []*openRow {
	if e.memory == nil {
		e.Stats.Reviewed = countRows(rows)
		return rows
	}

	keys := make([]string, 0, len(rows)*3)
	for _, r := range rows {
		keys = append(keys, decisionKey(r))
		if r.normName != "" {
			keys = append(keys, r.normName, "manual:"+r.normName)
		}
	}
	cached, err := e.memory.LookupDecisions(ctx, keys)
	if err != nil {
		cached = nil // a cache miss is never fatal
	}

	pending := make([]*openRow, 0, len(rows))
	for _, r := range rows {
		d, ok := cached[decisionKey(r)]
		if !ok && r.normName != "" {
			if md, mok := cached["manual:"+r.normName]; mok {
				d, ok = md, true
			} else if nd, nok := cached[r.normName]; nok {
				d, ok = nd, true
			}
		}
		if !ok {
			pending = append(pending, r)
			continue
		}
		e.Stats.CacheHits += len(r.sourceRows)
		// The guard runs on a remembered answer too. The catalogue moves
		// between imports and a decision that was sound in March can conflict
		// in June.
		j := matchflow.Judgement{
			Settled:    r.settled,
			Current:    r.guess,
			Offered:    true, // it was offered when the answer was bought
			Confidence: d.Confidence,
			Floor:      ceilings.MinApplyConfidence,
		}
		if d.ChosenProductID != nil && *d.ChosenProductID > 0 {
			j.Conflicts = !e.index.IdentityConflict(r.row, *d.ChosenProductID).None()
		}
		e.record(matchflow.Verdict(j, d.ChosenProductID), r,
			d.Confidence, d.ChosenProductID, d.Reason)
	}
	e.Stats.Reviewed = countRows(pending)
	return pending
}

// flush persists the decisions and the aliases they imply.
func (e *Enhancement) flush(ctx context.Context, decisions []CachedDecision) {
	if e.memory == nil || len(decisions) == 0 {
		return
	}
	// A cache write failing must not fail the run: the decisions were still
	// applied, they will simply be paid for again next time.
	_ = e.memory.SaveDecisions(ctx, decisions)

	// Each accepted match is also recorded as an *untrusted* alias, source
	// 'ai_confirmed', which the deterministic alias tier deliberately excludes.
	// The row exists so a person accepting the match can promote it and so an
	// operator can see what the model has been deciding — not so the next
	// import trusts it. One confident mistake propagating silently to every
	// vendor is precisely what this guards against.
	for _, d := range decisions {
		if d.ChosenProductID == nil || d.NormName == "" {
			continue
		}
		_ = e.memory.SaveAlias(ctx, *d.ChosenProductID, d.NormName, "ai_confirmed", d.Confidence)
	}
}
