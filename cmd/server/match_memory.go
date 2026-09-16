package main

// One decision cache, four tools.
//
// catalog.match_decisions has always been one table, and the vendor import and
// the smart order have always shared it — an answer paid for by a pharmacy's
// order is free to the vendor whose price list asks the same question. The
// other two import paths were not in that arrangement: the saving-list import
// and the administrator's master-catalogue import each paid for every question
// on every upload, asking the same model the same thing through the same
// prompt, and filing the answer nowhere.
//
// The obstacle was vocabulary rather than storage. Each module declares its own
// CachedDecision type so that none of them has to import another, which is
// right; the consequence was that a third tool could only join by declaring a
// third. matchflow.Memory is that vocabulary, and this adapter is the one
// translation between it and the repository that owns the table.

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// decisionStore is the half of the ingest repository that talks to the cache.
//
// Declared as an interface rather than taking the concrete repository so this
// file states exactly what it needs, and so a test can supply it.
type decisionStore interface {
	LookupDecisions(ctx context.Context, keys []string) (map[string]ingest.CachedDecision, error)
	SaveDecisions(ctx context.Context, decisions []ingest.CachedDecision) error
	SaveAlias(ctx context.Context, productID int64, alias, source string, confidence float64) error
}

// matchMemory adapts the ingest repository to the shared port.
type matchMemory struct{ store decisionStore }

func newMatchMemory(store decisionStore) matchflow.Memory {
	if store == nil {
		return nil
	}
	return &matchMemory{store: store}
}

func (m *matchMemory) Lookup(
	ctx context.Context, keys []string,
) (map[string]matchflow.Remembered, error) {
	found, err := m.store.LookupDecisions(ctx, keys)
	if err != nil {
		return nil, err
	}
	out := make(map[string]matchflow.Remembered, len(found))
	for key, d := range found {
		out[key] = matchflow.Remembered{
			Key: d.Key, NormName: d.NormName, ChosenProductID: d.ChosenProductID,
			Confidence: d.Confidence, Reason: d.Reason, PromptVersion: d.PromptVersion,
			Scope: d.Scope, Source: d.Source,
		}
	}
	return out, nil
}

func (m *matchMemory) Save(ctx context.Context, decisions []matchflow.Remembered) error {
	if len(decisions) == 0 {
		return nil
	}
	out := make([]ingest.CachedDecision, 0, len(decisions))
	for _, d := range decisions {
		out = append(out, ingest.CachedDecision{
			Key: d.Key, NormName: d.NormName, ChosenProductID: d.ChosenProductID,
			Confidence: d.Confidence, Reason: d.Reason, PromptVersion: d.PromptVersion,
			Scope: d.Scope, Source: d.Source,
		})
	}
	return m.store.SaveDecisions(ctx, out)
}

func (m *matchMemory) SaveAlias(
	ctx context.Context, productID int64, alias, source string, confidence float64,
) error {
	return m.store.SaveAlias(ctx, productID, alias, source, confidence)
}
