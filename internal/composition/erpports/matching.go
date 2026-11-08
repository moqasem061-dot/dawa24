package erpports

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// The matching adapters wrap the matching.Service interface value (no pointer,
// matching/internal/catalog/matching/service.go:22), field conversion only.

// NewMatcher adapts the deterministic Prepare stage.
func NewMatcher(svc matching.Service) erp.Matcher {
	return matchingMatcher{svc: svc}
}

type matchingMatcher struct {
	svc matching.Service
}

func (m matchingMatcher) Prepare(ctx context.Context, req productmatch.PrepareRequest) (productmatch.PrepareResult, error) {
	return m.svc.Prepare(ctx, req)
}

// NewMatchMemory adapts the knowledge store (catalog.match_knowledge).
func NewMatchMemory(svc matching.Service) erp.MatchMemory {
	return matchingMemory{svc: svc}
}

type matchingMemory struct {
	svc matching.Service
}

func (m matchingMemory) Remember(ctx context.Context, sc productmatch.Scope, k erp.KnowledgeWrite) error {
	return m.svc.Remember(ctx, sc, matching.KnowledgeWrite{
		SourceNamespace:   k.SourceNamespace,
		QuestionSignature: k.QuestionSignature,
		Kind:              k.Kind,
		TargetProductID:   k.TargetProductID,
		TargetGranularity: k.TargetGranularity,
		PolicyVersion:     k.PolicyVersion,
		NormalizerVersion: k.NormalizerVersion,
		DisplayText:       k.DisplayText,
	})
}

var _ erp.Matcher = matchingMatcher{}
var _ erp.MatchMemory = matchingMemory{}
