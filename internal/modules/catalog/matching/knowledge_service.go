package matching

import (
	"context"
	"fmt"
	"strconv"

	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// Remember persists a match knowledge entry for the effective scope (US8 T110, T114).
func (s *serviceImpl) Remember(ctx context.Context, sc Scope, k KnowledgeWrite) error {
	if s.repo == nil {
		return ErrNotImplemented
	}
	var orgIDPtr *int64
	if sc.EffectiveOrg > 0 {
		orgIDPtr = &sc.EffectiveOrg
	}
	return s.repo.Upsert(ctx, Knowledge{
		OrganizationID:    orgIDPtr,
		SourceNamespace:   k.SourceNamespace,
		QuestionSignature: k.QuestionSignature,
		Kind:              k.Kind,
		TargetProductID:   k.TargetProductID,
		TargetGranularity: k.TargetGranularity,
		PolicyVersion:     k.PolicyVersion,
		NormalizerVersion: k.NormalizerVersion,
		DisplayText:       k.DisplayText,
		Status:            "active",
		CreatedBy:         strconv.FormatInt(sc.ActorUserID, 10),
	})
}

// ListKnowledge queries persisted match knowledge records with filtering and pagination.
func (s *serviceImpl) ListKnowledge(ctx context.Context, sc Scope, f KnowledgeFilter) (Page[Knowledge], error) {
	if s.repo == nil {
		return Page[Knowledge]{}, ErrNotImplemented
	}
	return s.repo.List(ctx, f)
}

// RevokeKnowledge invalidates a match knowledge entry.
func (s *serviceImpl) RevokeKnowledge(ctx context.Context, sc Scope, id int64) error {
	if s.repo == nil {
		return ErrNotImplemented
	}
	actorID := strconv.FormatInt(sc.ActorUserID, 10)
	return s.repo.Revoke(ctx, id, actorID)
}

// PromoteKnowledge elevates an org-scoped knowledge record to platform scope,
// stripping raw display text to normalized catalogue-safe text (US8 T113).
func (s *serviceImpl) PromoteKnowledge(ctx context.Context, sc Scope, id int64) error {
	if s.repo == nil {
		return ErrNotImplemented
	}
	items, err := s.repo.List(ctx, KnowledgeFilter{Limit: 100})
	if err != nil {
		return err
	}
	actorID := strconv.FormatInt(sc.ActorUserID, 10)
	for _, item := range items.Items {
		if item.ID == id {
			return s.repo.Upsert(ctx, Knowledge{
				OrganizationID:        nil, // Platform scope
				SourceNamespace:       item.SourceNamespace,
				QuestionSignature:     item.QuestionSignature,
				Kind:                  item.Kind,
				TargetProductID:       item.TargetProductID,
				TargetGranularity:     item.TargetGranularity,
				TargetFactFingerprint: item.TargetFactFingerprint,
				PolicyVersion:         item.PolicyVersion,
				NormalizerVersion:     item.NormalizerVersion,
				DisplayText:           productmatch.NormalizeText(item.DisplayText),
				Status:                "active",
				CreatedBy:             actorID,
			})
		}
	}
	return fmt.Errorf("knowledge record %d not found", id)
}
