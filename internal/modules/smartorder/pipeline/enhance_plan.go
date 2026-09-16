package pipeline

// Turning a run's rows into questions.
//
// The packing itself — the shared catalogue window, the byte budget, the
// de-duplication of identical questions, the order the budget is spent in —
// lives in internal/shared/matchflow, because the smart order, the vendor
// import, the saving list and the master-catalogue import all did it and the
// four copies had drifted. This file is only the translation between a smart
// order's vocabulary and that one.
//
// What it adds is the second population. The stage used to see only the rows
// the deterministic engine could not settle, so the engine's CONFIDENT mistakes
// were the one class of error in the file that nothing ever looked at twice.
// Now every row whose match rests on a NAME goes to the model — the ones it
// could not settle, to be resolved, and the ones it did settle, to be checked.
//
// Rows settled on an identifier do not go. A barcode is the same physical
// package and the buyer's own confirmed mapping is their own assertion about
// their own vocabulary; a model cannot improve on either, and asking costs the
// budget that the ambiguous rows need.

import (
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// verifiable reports whether a settled line's match rests on a name, and is
// therefore worth a second opinion.
//
// The identifier tiers are excluded by name rather than by confidence, because
// confidence is what the tier asserted and every one of them asserts 1.0.
func verifiable(l *smartorder.Line) bool {
	switch l.MatchMethod {
	case smartorder.MethodExactName, smartorder.MethodFuzzy,
		smartorder.MethodIdentityKey, smartorder.MethodAlias:
		return true
	}
	return false
}

// questions renders the run's reviews as questions for the model.
func (e *Enhancement) questions(reviews []Review) []matchflow.Question {
	out := make([]matchflow.Question, 0, len(reviews))
	for _, r := range reviews {
		window := make([]matchflow.CatalogEntry, 0, len(r.Candidates))
		ids := make([]int64, 0, len(r.Candidates))
		for _, c := range r.Candidates {
			window = append(window, e.describe(c.ProductID))
			ids = append(ids, c.ProductID)
		}
		// A settled row's own product must be on the table even when retrieval
		// did not rank it, or a "check" item asks the model to confirm an id it
		// was never shown — and an answer naming a product outside the window
		// is refused as a hallucination.
		if r.Settled && r.Line.MatchedProductID != nil {
			window, ids = includeCurrent(window, ids, *r.Line.MatchedProductID, e)
		}

		out = append(out, matchflow.Question{
			Key:    matchflow.DecisionKey(r.Line.NormName, ids),
			Window: window,
			Item: matchflow.Item{
				Text:         r.Line.RawName,
				Brand:        r.Row.Name,
				Strength:     r.Row.Concentration,
				DosageForm:   r.Row.DosageForm,
				PackSize:     r.Row.PackSize,
				Manufacturer: r.Row.Manufacturer,
				Scientific:   r.Row.Scientific,
				SKU:          r.Line.RawSKU,
				Barcode:      r.Line.RawBarcode,
				CurrentGuess: r.Line.MatchedProductID,
				CurrentScore: r.Line.MatchConfidence,
				Settled:      r.Settled,
			},
			Risk: matchflow.Risk(r.Settled, r.Ambiguous, r.Line.MatchConfidence),
		})
	}
	return out
}

// includeCurrent puts a settled row's own product into its window if retrieval
// left it out.
func includeCurrent(window []matchflow.CatalogEntry, ids []int64, id int64,
	e *Enhancement) ([]matchflow.CatalogEntry, []int64) {
	for _, existing := range ids {
		if existing == id {
			return window, ids
		}
	}
	return append(window, e.describe(id)), append(ids, id)
}

// describe projects a catalogue product for the window.
//
// It reads the in-memory index rather than the shortlist entry, because the
// index carries the English name and the shortlist does not — and the English
// name is what settles the transliteration cases this stage exists for.
func (e *Enhancement) describe(id int64) matchflow.CatalogEntry {
	p, ok := e.index.Lookup(id)
	if !ok {
		return matchflow.CatalogEntry{ProductID: id}
	}
	return matchflow.CatalogEntry{
		ProductID:     id,
		NameAR:        p.NameAR,
		NameEN:        p.NameEN,
		Scientific:    p.Scientific,
		DosageForm:    p.DosageForm,
		Concentration: p.Concentration,
		Manufacturer:  p.Manufacturer,
	}
}

// byKey groups the run's rows by the question each of them asks, so one answer
// settles every row that asked it.
//
// A file that lists the same product across three warehouses asks one question
// and pays for it once. On the live twenty-five-thousand-row price lists this
// stage was built for, the distinct-question count is routinely a third of the
// row count.
func byKey(reviews []Review) map[string][]Review {
	out := make(map[string][]Review, len(reviews))
	for _, r := range reviews {
		out[keyOf(r)] = append(out[keyOf(r)], r)
	}
	return out
}

// keyOf identifies the exact question a row asks.
//
// The retrieved candidate ids are part of it, so an answer is only reused when
// the same options were on the table; a decision made against a different
// shortlist answers a question nobody asked. PromptVersion is folded in by
// DecisionKey, so a prompt change invalidates cleanly rather than silently.
func keyOf(r Review) string {
	ids := make([]int64, 0, len(r.Candidates)+1)
	for _, c := range r.Candidates {
		ids = append(ids, c.ProductID)
	}
	if r.Settled && r.Line.MatchedProductID != nil {
		ids = appendMissing(ids, *r.Line.MatchedProductID)
	}
	return matchflow.DecisionKey(r.Line.NormName, ids)
}

func appendMissing(ids []int64, id int64) []int64 {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

// DebugDecisionKey exposes the cache key for the cross-module test that asserts
// this pipeline and the vendor import hash the same question identically.
func DebugDecisionKey(normName string, candidateIDs []int64) string {
	return matchflow.DecisionKey(normName, candidateIDs)
}
