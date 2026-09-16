package ui

// Turning a saving list's rows into questions.
//
// saving_products_ai.go runs the stage and judges the answers. This file builds
// what is asked: one question per row worth asking about, each carrying its OWN
// retrieved window, which the shared planner then de-duplicates into one
// catalogue block per request.
//
// That last point is the defect this split was made during. The window used to
// be built once for the whole file and attached to every request, so a long
// list paid for its entire catalogue window on every call.

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// savingQuestion is one staged row with the retrieval that justifies asking.
type savingQuestion struct {
	targets  []*StagedSavingItem
	row      *productmatch.Row
	question matchflow.Question
	// settled says the deterministic tiers already linked this row, so the
	// question is verification rather than resolution.
	settled bool
	// current is the product they linked it to.
	current *int64
	// answered marks a question the decision cache settled, so it is not asked
	// again in the same run.
	answered bool
}

func (q *savingQuestion) firstTarget() *StagedSavingItem {
	if q != nil && len(q.targets) > 0 {
		return q.targets[0]
	}
	return nil
}

// offers reports whether a product id was among this row's retrieved options.
func (q *savingQuestion) offers(id int64) bool {
	for _, opt := range q.question.Item.Options {
		if opt == id {
			return true
		}
	}
	for _, e := range q.question.Window {
		if e.ProductID == id {
			return true
		}
	}
	return false
}

// planSavingQuestions retrieves candidates for every row worth asking about and
// builds its question.
//
// Each question carries its OWN window. The planner de-duplicates those into
// one shared catalogue block per request, which is the point: a window built
// once for the whole file and attached to every request is the same rows sent
// thirty times.
func planSavingQuestions(idx *productmatch.Index, items []*StagedSavingItem) map[string]*savingQuestion {
	recall := productmatch.DefaultRecallOptions()
	recall.Limit = savingAICeilings.RecallLimit

	out := make(map[string]*savingQuestion, len(items))
	for _, it := range items {
		if it == nil || strings.TrimSpace(it.NameProduct) == "" {
			continue
		}
		settled := it.ProductID != nil && *it.ProductID > 0
		if settled && !savingVerifiable(it.MatchType) {
			continue
		}

		row := &productmatch.Row{Name: it.NameProduct, SKU: it.SKU}
		candidates := idx.Recall(row, recall)

		window := make([]matchflow.CatalogEntry, 0, len(candidates)+1)
		options := make([]int64, 0, len(candidates)+1)
		plausible := false
		for _, c := range candidates {
			if c.Score >= savingAICeilings.MinPlausible {
				plausible = true
			}
			options = append(options, c.ProductID)
			if p, ok := idx.Lookup(c.ProductID); ok {
				window = append(window, matchflow.CatalogEntry{
					ProductID: p.ID, NameAR: p.NameAR, NameEN: p.NameEN,
					Scientific: p.Scientific, DosageForm: p.DosageForm,
					Concentration: p.Concentration, Manufacturer: p.Manufacturer,
				})
			}
		}
		// A settled row's own product must be on the table even when retrieval
		// did not rank it, or the model is asked to confirm an id it was never
		// shown — and an answer naming a product outside the window is refused
		// as a hallucination.
		if settled && !offersID(options, *it.ProductID) {
			if p, ok := idx.Lookup(*it.ProductID); ok {
				options = append(options, p.ID)
				window = append(window, matchflow.CatalogEntry{
					ProductID: p.ID, NameAR: p.NameAR, NameEN: p.NameEN,
					Scientific: p.Scientific, DosageForm: p.DosageForm,
					Concentration: p.Concentration, Manufacturer: p.Manufacturer,
				})
				plausible = true
			}
		}
		if !plausible {
			// Nothing in the catalogue is close. Unlinked is already the honest
			// answer and no model can improve on it.
			continue
		}

		key := matchflow.DecisionKey(productmatch.NormalizeText(it.NameProduct), options)
		if existing, dup := out[key]; dup {
			existing.targets = append(existing.targets, it)
			continue
		}

		q := &savingQuestion{
			targets: []*StagedSavingItem{it},
			row:     row,
			settled: settled,
			current: it.ProductID,
			question: matchflow.Question{
				Key:    key,
				Window: window,
				Item: matchflow.Item{
					Text:         it.NameProduct,
					SKU:          it.SKU,
					Options:      options,
					CurrentGuess: it.ProductID,
					CurrentScore: it.Confidence,
					Settled:      settled,
				},
				Risk: matchflow.Risk(settled, false, it.Confidence),
			},
		}
		out[key] = q
	}
	return out
}

// savingVerifiable reports whether a linked row's link rests on a name or AI,
// and is therefore worth a second opinion or confirmation.
//
// A barcode is the same physical package, an id the file stated outright is the
// pharmacy's own assertion, and a catalogue code they mapped themselves is too.
// A model is invoked for name-based matches (exact, normalized, core, or fuzzy)
// or AI-derived links to verify identity and correct nuances like strength/dosage.
func savingVerifiable(matchType string) bool {
	switch matchType {
	case "fuzzy_name", "exact_name", "norm_name", "core_name", savingMatchTypeAI:
		return true
	}
	return false
}

// offersID reports whether an id is already among a row's retrieved options.
func offersID(ids []int64, id int64) bool {
	for _, existing := range ids {
		if existing == id {
			return true
		}
	}
	return false
}
