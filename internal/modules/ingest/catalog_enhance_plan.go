package ingest

// Turning a run's rows into questions.
//
// The packing itself — the shared catalogue window, the byte budget, the
// collapsing of identical questions, the order the budget is spent in — lives
// in internal/shared/matchflow, because the smart order, the saving list and
// the master-catalogue import all did it too and the four copies had drifted.
// Only one of them enforced the byte budget; only one collapsed duplicates;
// none of them ordered the work, which did not matter while the stage saw only
// a residue and matters a great deal now that it sees whole files.
//
// This file is the translation between a vendor import's vocabulary and that
// one.
//
// Supplier files are where the shared window pays most. A distributor's price
// list is nine thousand rows of a few hundred molecules: the rows retrieve
// heavily overlapping catalogue products, so the twentieth antihypertensive
// costs its item line and nothing else.

import (
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// questions renders the open rows as questions for the model.
func (e *Enhancement) questions(rows []*openRow) []matchflow.Question {
	out := make([]matchflow.Question, 0, len(rows))
	for _, r := range rows {
		window := make([]matchflow.CatalogEntry, 0, len(r.candidates))
		for _, c := range r.candidates {
			window = append(window, e.describe(c.ProductID))
		}
		// A settled row's own product must be on the table even when retrieval
		// did not rank it, or the model is asked to confirm an id it was never
		// shown — and an answer naming a product outside the window is refused
		// as a hallucination.
		if r.settled && r.guess != nil {
			window = includeGuess(window, *r.guess, e)
		}

		out = append(out, matchflow.Question{
			Key:    decisionKey(r),
			Window: window,
			Item: matchflow.Item{
				Text:         r.row.DisplayName(),
				Brand:        r.row.Name,
				Strength:     r.row.Concentration,
				DosageForm:   r.row.DosageForm,
				PackSize:     r.row.PackSize,
				Manufacturer: r.row.Manufacturer,
				Scientific:   r.row.Scientific,
				SKU:          r.row.SKU,
				Barcode:      r.row.Barcode,
				CurrentGuess: r.guess,
				CurrentScore: r.score,
				Settled:      r.settled,
			},
			Risk: matchflow.Risk(r.settled, r.ambiguous, r.score),
		})
	}
	return out
}

// includeGuess puts a settled row's own product into its window if retrieval
// left it out.
func includeGuess(window []matchflow.CatalogEntry, id int64, e *Enhancement) []matchflow.CatalogEntry {
	for _, entry := range window {
		if entry.ProductID == id {
			return window
		}
	}
	return append(window, e.describe(id))
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

// decisionKey identifies the exact question one row asks.
//
// Byte-identical to the key the smart order and the saving list compute for the
// same question, which is the whole point: the tools file into and read from
// one table, so an answer bought once is free wherever the same product is
// written the same way against the same shortlist.
func decisionKey(r *openRow) string {
	ids := make([]int64, 0, len(r.candidates)+1)
	for _, c := range r.candidates {
		ids = append(ids, c.ProductID)
	}
	if r.settled && r.guess != nil {
		ids = appendMissing(ids, *r.guess)
	}
	return matchflow.DecisionKey(r.normName, ids)
}

func appendMissing(ids []int64, id int64) []int64 {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

// byQuestion indexes the open rows by the question each asks, so an answer can
// be routed back to whichever row asked it.
func byQuestion(rows []*openRow) map[string]*openRow {
	out := make(map[string]*openRow, len(rows))
	for _, r := range rows {
		out[decisionKey(r)] = r
	}
	return out
}

// DebugDecisionKey exposes the cache key for the cross-module test that asserts
// this importer and the smart order hash the same question identically.
func DebugDecisionKey(normName string, candidateIDs []int64) string {
	return matchflow.DecisionKey(normName, candidateIDs)
}

// countRows totals the spreadsheet rows behind a set of questions.
func countRows(rows []*openRow) int {
	n := 0
	for _, r := range rows {
		n += len(r.sourceRows)
	}
	return n
}

// firstRow is the spreadsheet row that first asked this question.
func (r *openRow) firstRow() int {
	if len(r.sourceRows) == 0 {
		return 0
	}
	return r.sourceRows[0]
}
