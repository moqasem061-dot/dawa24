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

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// decisionStore is the half of the ingest repository that talks to the legacy cache for reads.
type decisionStore interface {
	LookupDecisions(ctx context.Context, keys []string) (map[string]ingest.CachedDecision, error)
}

// matchMemory adapts matching.Service and ingest repository to the shared port.
type matchMemory struct {
	store    decisionStore
	matchSvc matching.Service
}

func newMatchMemory(store decisionStore, matchSvc matching.Service) matchflow.Memory {
	return &matchMemory{store: store, matchSvc: matchSvc}
}

func (m *matchMemory) Lookup(
	ctx context.Context, keys []string,
) (map[string]matchflow.Remembered, error) {
	out := make(map[string]matchflow.Remembered)

	// 1. Query match_knowledge via matching.Service
	if m.matchSvc != nil {
		var orgID int64
		if actor, ok := authctx.From(ctx); ok {
			orgID = actor.OrganizationID
		} else if tid, ok := database.TenantFrom(ctx); ok {
			orgID = tid
		}
		sc := matching.Scope{
			EffectiveOrg: orgID,
			Workflow:     matching.WorkflowCatalog,
			EntryPoint:   "matchflow.memory.lookup",
		}
		for _, k := range keys {
			res, err := m.matchSvc.ListKnowledge(ctx, sc, matching.KnowledgeFilter{
				Search: k,
				Status: "active",
				Limit:  1,
			})
			if err == nil && len(res.Items) > 0 {
				item := res.Items[0]
				scope := "platform"
				if item.OrganizationID != nil {
					scope = "org"
				}
				pVer := ""
				if item.PromptVersion != nil {
					pVer = *item.PromptVersion
				}
				out[k] = matchflow.Remembered{
					Key:             k,
					NormName:        item.DisplayText,
					ChosenProductID: item.TargetProductID,
					Confidence:      1.0,
					Reason:          item.Status,
					PromptVersion:   pVer,
					Scope:           scope,
					Source:          item.Kind,
				}
			}
		}
	}

	// 2. Fallback to read-only legacy match_decisions for missing keys
	missing := make([]string, 0, len(keys))
	for _, k := range keys {
		if _, ok := out[k]; !ok {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 && m.store != nil {
		found, err := m.store.LookupDecisions(ctx, missing)
		if err == nil {
			for key, d := range found {
				if _, ok := out[key]; !ok {
					out[key] = matchflow.Remembered{
						Key: d.Key, NormName: d.NormName, ChosenProductID: d.ChosenProductID,
						Confidence: d.Confidence, Reason: d.Reason, PromptVersion: d.PromptVersion,
						Scope: d.Scope, Source: d.Source,
					}
				}
			}
		}
	}

	return out, nil
}

func (m *matchMemory) Save(ctx context.Context, decisions []matchflow.Remembered) error {
	if len(decisions) == 0 || m.matchSvc == nil {
		return nil
	}
	var orgID, userID int64
	if actor, ok := authctx.From(ctx); ok {
		orgID = actor.OrganizationID
		userID = actor.UserID
	} else if tid, ok := database.TenantFrom(ctx); ok {
		orgID = tid
	}
	sc := matching.Scope{
		EffectiveOrg:    orgID,
		ActorUserID:     userID,
		SourceNamespace: "catalog",
		Workflow:        matching.WorkflowCatalog,
		EntryPoint:      "matchflow.memory.save",
	}
	for _, d := range decisions {
		kind := "ai_answer"
		if d.Source == "manual" || d.Source == "admin" || d.Source == "human_confirm" {
			kind = "human_confirm"
		}
		kw := matching.KnowledgeWrite{
			SourceNamespace:   "catalog",
			QuestionSignature: d.Key,
			Kind:              kind,
			TargetProductID:   d.ChosenProductID,
			PolicyVersion:     "v1",
			NormalizerVersion: productmatch.NormalizerVersion,
			DisplayText:       d.NormName,
		}
		_ = m.matchSvc.Remember(ctx, sc, kw)
	}
	return nil
}

func (m *matchMemory) SaveAlias(
	ctx context.Context, productID int64, alias, source string, confidence float64,
) error {
	if m.matchSvc == nil || productID <= 0 || alias == "" {
		return nil
	}
	var orgID, userID int64
	if actor, ok := authctx.From(ctx); ok {
		orgID = actor.OrganizationID
		userID = actor.UserID
	} else if tid, ok := database.TenantFrom(ctx); ok {
		orgID = tid
	}
	norm := productmatch.NormalizeText(alias)
	sig := productmatch.QuestionSignature(productmatch.IdentityFacts{}, norm, productmatch.NormalizerVersion)
	sc := matching.Scope{
		EffectiveOrg:    orgID,
		ActorUserID:     userID,
		SourceNamespace: "catalog",
		Workflow:        matching.WorkflowCatalog,
		EntryPoint:      "matchflow.memory.alias",
	}
	kw := matching.KnowledgeWrite{
		SourceNamespace:   "catalog",
		QuestionSignature: sig,
		Kind:              "curated_alias",
		TargetProductID:   &productID,
		PolicyVersion:     "v1",
		NormalizerVersion: productmatch.NormalizerVersion,
		DisplayText:       norm,
	}
	return m.matchSvc.Remember(ctx, sc, kw)
}
