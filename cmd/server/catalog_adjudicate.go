package main

// The administrator's catalogue import asks the same question as everyone else.
//
// It used to ask it with its own system prompt, its own response schema and its
// own wire types, in internal/modules/catalog/gateway/adjudicate.go — a third
// phrasing of "which of these catalogue products is this row, or none of them?"
// alongside the smart order's and the vendor import's. Three prompts is three
// things to tune, three ways to drift, and three populations in a decision cache
// that is keyed by the prompt's version.
//
// This adapter translates the catalogue module's port onto the shared
// capability, so the module keeps its own vocabulary and the platform keeps one
// prompt. The translation is mechanical and lives here because this is the only
// place allowed to know about both.

import (
	"context"
	"sort"

	"github.com/muhiya/dawa24-store/internal/modules/aicapabilities"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

type catalogAdjudicateAdapter struct {
	caps *aicapabilities.Service
}

// AdjudicateMatches answers a batch of ambiguous catalogue rows.
//
// The catalogue module hands each row its own shortlist. The shared capability
// takes one de-duplicated window that every row may answer from, which is
// strictly more information for the same tokens — and it repairs the retrieval
// failure the per-row shape cannot: the right product was retrieved, for the row
// above.
//
// What it may answer with is still bounded. import_match.go rejects any product
// that was not on *that row's* shortlist before applying it, so widening the
// window widens what the model can see without widening what it can do.
func (a *catalogAdjudicateAdapter) AdjudicateMatches(
	ctx context.Context, req catalog.MatchAdjudicationRequest,
) ([]catalog.MatchAdjudicationResult, error) {
	if len(req.Items) == 0 {
		return nil, nil
	}

	batch := matchflow.Batch{Items: make([]matchflow.Item, 0, len(req.Items))}
	seen := make(map[int64]bool)
	// refs maps the request-local index back to the caller's own row reference,
	// which is an int64 here and an int in the shared contract.
	refs := make([]int64, 0, len(req.Items))

	for _, it := range req.Items {
		options := make([]int64, 0, len(it.Candidates))
		for _, c := range it.Candidates {
			options = append(options, c.ProductID)
			if seen[c.ProductID] {
				continue
			}
			seen[c.ProductID] = true
			batch.Catalog = append(batch.Catalog, matchflow.CatalogEntry{
				ProductID:     c.ProductID,
				NameAR:        c.Name,
				NameEN:        c.NameEN,
				Scientific:    c.Scientific,
				DosageForm:    c.DosageForm,
				Concentration: c.Concentration,
				Manufacturer:  c.Manufacturer,
			})
		}
		batch.Items = append(batch.Items, matchflow.Item{
			Ref:          len(refs),
			Text:         it.Text,
			Options:      options,
			CurrentGuess: it.CurrentGuess,
			CurrentScore: it.CurrentScore,
			Settled:      it.Settled,
		})
		refs = append(refs, it.Ref)
	}

	sort.SliceStable(batch.Catalog, func(i, j int) bool {
		return batch.Catalog[i].ProductID < batch.Catalog[j].ProductID
	})

	batch.Feature = matchflow.FeatureCatalogImport
	decisions, err := a.caps.EnhanceMatches(ctx, batch)
	if err != nil {
		return nil, err
	}

	out := make([]catalog.MatchAdjudicationResult, 0, len(decisions))
	for _, d := range decisions {
		if d.Ref < 0 || d.Ref >= len(refs) {
			continue
		}
		out = append(out, catalog.MatchAdjudicationResult{
			Ref:        refs[d.Ref],
			ProductID:  d.ProductID,
			Confidence: d.Confidence,
			Reason:     d.Reason,
		})
	}
	return out, nil
}
