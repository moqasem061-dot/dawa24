package main

// Wiring the vendor catalogue import's AI stage.
//
// The translation is mechanical and lives here rather than in either module,
// because this is the only place allowed to know about both. It is the same
// capability, the same system prompt and the same response contract the smart
// order runs through — deliberately, so the two share one prompt to tune and
// one decision cache to hit.

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/aicapabilities"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

type ingestEnhanceAdapter struct {
	caps *aicapabilities.Service
}

// Enhance satisfies matchflow.Enhancer, which is the shared shape every
// importer's AI stage asks through.
//
// The conversions below are gone because the types are the same types now: the
// batch, the window entry, the item and the decision are all declared once in
// internal/shared/matchflow and aliased everywhere else. This method is the whole
// adapter for any caller that speaks the shared contract.
func (a *ingestEnhanceAdapter) Enhance(
	ctx context.Context, batch matchflow.Batch,
) ([]matchflow.Decision, error) {
	return a.caps.EnhanceMatches(ctx, batch)
}

func (a *ingestEnhanceAdapter) EnhanceBatch(
	ctx context.Context, batch ingest.GatewayBatch,
) ([]ingest.GatewayOutcome, error) {
	req := aicapabilities.EnhanceRequest{
		Catalog: make([]aicapabilities.CatalogEntry, 0, len(batch.Catalog)),
		Items:   make([]aicapabilities.EnhanceItem, 0, len(batch.Items)),
		// Attribution for the AI usage ledger: the vendor reads i18n.TDefault("w4_mod.s_475_475") on its usage log, not a capability name.
		Feature: matchflow.FeatureVendorImport,
	}
	for _, c := range batch.Catalog {
		req.Catalog = append(req.Catalog, aicapabilities.CatalogEntry(c))
	}
	for _, it := range batch.Items {
		req.Items = append(req.Items, aicapabilities.EnhanceItem(it))
	}

	decisions, err := a.caps.EnhanceMatches(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make([]ingest.GatewayOutcome, 0, len(decisions))
	for _, d := range decisions {
		out = append(out, ingest.GatewayOutcome(d))
	}
	return out, nil
}
