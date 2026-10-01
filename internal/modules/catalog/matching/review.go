package matching

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// Review executes a reviewer action (select, correct, reject, exclude, undo) on a row.
func (s *serviceImpl) Review(ctx context.Context, sc Scope, a ReviewAction) (matchflow.RowDecision, error) {
	if sc.Workflow == "" {
		return matchflow.RowDecision{}, errors.New("matching: scope workflow is required")
	}

	// Optimistic concurrency check (FR-048)
	if a.ExpectedRev != a.CurrentDec.Revision {
		return matchflow.RowDecision{}, ErrRevisionConflict
	}

	catRev, err := s.repo.CurrentRevision(ctx, "master")
	if err != nil {
		catRev = 1
	}

	idx, err := s.indexCache.Get(ctx, "master", catRev)
	if err != nil || idx == nil {
		return matchflow.RowDecision{}, errors.New("matching: catalog unavailable")
	}

	pol, err := s.repo.ActivePolicy(ctx, string(sc.Workflow), sc.EffectiveOrg)
	if err != nil || pol.Version == "" {
		pol = productmatch.DefaultPolicy()
	}

	var dec matchflow.RowDecision
	now := time.Now().UTC()
	actorID := strconv.FormatInt(sc.ActorUserID, 10)

	switch a.Action {
	case "select":
		if a.TargetID <= 0 {
			return matchflow.RowDecision{}, errors.New("matching: target_id is required for select")
		}
		var err error
		dec, err = confirmTarget(idx, pol, a, a.TargetID, "reviewer_selected", catRev, actorID, now)
		if err != nil {
			return matchflow.RowDecision{}, err
		}

	case "correct":
		if a.TargetID > 0 {
			var err error
			dec, err = confirmTarget(idx, pol, a, a.TargetID, "reviewer_corrected", catRev, actorID, now)
			if err != nil {
				return matchflow.RowDecision{}, err
			}
		} else {
			dec = idx.Decide(&a.Row, pol)
			dec.Revision = a.ExpectedRev + 1
			dec.Basis = matchflow.BasisReviewer
			dec.Actor = &matchflow.DecisionActor{
				UserID:      actorID,
				At:          now,
				Corrections: a.Corrections,
			}
		}

	case "reject":
		dec = a.CurrentDec
		dec.V = 1
		dec.State = matchflow.StateNoCandidate
		dec.ProductID = nil
		dec.Basis = matchflow.BasisReviewer
		dec.Reasons = append(dec.Reasons, matchflow.Reason{Code: "reviewer_rejected"})
		dec.Revision = a.ExpectedRev + 1
		dec.Actor = &matchflow.DecisionActor{
			UserID: actorID,
			At:     now,
		}

	case "exclude":
		dec = a.CurrentDec
		dec.V = 1
		dec.State = matchflow.StateExcluded
		dec.ProductID = nil
		dec.Basis = matchflow.BasisReviewer
		reasonCode := "reviewer_excluded"
		if a.Reason != "" {
			reasonCode = a.Reason
		}
		dec.Reasons = []matchflow.Reason{{Code: reasonCode}}
		dec.Revision = a.ExpectedRev + 1
		dec.Actor = &matchflow.DecisionActor{
			UserID: actorID,
			At:     now,
		}

	case "undo":
		dec = idx.Decide(&a.Row, pol)
		dec.Revision = a.ExpectedRev + 1
		dec.Actor = nil

	default:
		return matchflow.RowDecision{}, fmt.Errorf("matching: unknown review action %q", a.Action)
	}

	// Persist human confirmation/rejection into match_knowledge (US8 T110)
	s.recordReviewKnowledge(ctx, sc, a, dec, pol, idx)

	slog.InfoContext(ctx, "matching: review action",
		"workflow", sc.Workflow,
		"effective_org", sc.EffectiveOrg,
		"actor_user_id", sc.ActorUserID,
		"action", a.Action,
		"target_id", a.TargetID,
		"expected_rev", a.ExpectedRev,
		"new_rev", dec.Revision,
		"result_state", dec.State,
	)

	return dec, nil
}

func (s *serviceImpl) recordReviewKnowledge(
	ctx context.Context,
	sc Scope,
	a ReviewAction,
	dec matchflow.RowDecision,
	pol productmatch.Policy,
	idx *productmatch.Index,
) {
	if s.repo == nil {
		return
	}
	actorID := strconv.FormatInt(sc.ActorUserID, 10)
	var orgIDPtr *int64
	if sc.EffectiveOrg > 0 {
		orgIDPtr = &sc.EffectiveOrg
	}

	switch dec.State {
	case matchflow.StateConfirmed:
		if dec.ProductID != nil && *dec.ProductID > 0 {
			rowCorrections := toProductMatchCorrections(a.Corrections)
			rowFacts := productmatch.ExtractRowFacts(&a.Row, rowCorrections)
			sig := productmatch.QuestionSignature(rowFacts, a.Row.Name, productmatch.NormalizerVersion)

			var fp string
			if idx != nil {
				if prod, ok := idx.Lookup(*dec.ProductID); ok && prod != nil {
					fp = ProductFactFingerprint(prod)
				}
			}

			_ = s.repo.Upsert(ctx, Knowledge{
				OrganizationID:        orgIDPtr,
				SourceNamespace:       string(sc.Workflow),
				QuestionSignature:     sig,
				Kind:                  "human_confirm",
				TargetProductID:       dec.ProductID,
				TargetGranularity:     string(dec.Granularity.Achieved),
				TargetFactFingerprint: fp,
				PolicyVersion:         pol.Version,
				NormalizerVersion:     productmatch.NormalizerVersion,
				DisplayText:           a.Row.DisplayName(),
				Status:                "active",
				CreatedBy:             actorID,
			})
		}

	case matchflow.StateNoCandidate:
		if a.Action == "reject" {
			rowFacts := productmatch.ExtractRowFacts(&a.Row, nil)
			sig := productmatch.QuestionSignature(rowFacts, a.Row.Name, productmatch.NormalizerVersion)
			_ = s.repo.Upsert(ctx, Knowledge{
				OrganizationID:    orgIDPtr,
				SourceNamespace:   string(sc.Workflow),
				QuestionSignature: sig,
				Kind:              "human_reject",
				TargetProductID:   nil,
				PolicyVersion:     pol.Version,
				NormalizerVersion: productmatch.NormalizerVersion,
				DisplayText:       a.Row.DisplayName(),
				Status:            "active",
				CreatedBy:         actorID,
			})
		}
	}
}

func confirmTarget(
	idx *productmatch.Index,
	pol productmatch.Policy,
	a ReviewAction,
	targetID int64,
	reasonCode string,
	catRev int64,
	actorID string,
	now time.Time,
) (matchflow.RowDecision, error) {
	p, ok := idx.Lookup(targetID)
	if !ok {
		return matchflow.RowDecision{}, fmt.Errorf("matching: product %d not found in catalog", targetID)
	}

	rowCorrections := toProductMatchCorrections(a.Corrections)
	rowFacts := productmatch.ExtractRowFacts(&a.Row, rowCorrections)
	comp := productmatch.CompareFacts(rowFacts, p.Facts())
	if len(comp.Hard) > 0 {
		return matchflow.RowDecision{}, fmt.Errorf("%w: %v", ErrHardConflict, comp.Hard)
	}

	achieved := matchflow.GranularityProduct
	packOutcome := matchflow.PackOutcomeAgree
	if pol.Required == productmatch.GranularityPack {
		if comp.Attributes["pack"] == productmatch.OutcomeAgree {
			achieved = matchflow.GranularityPack
		} else if comp.Attributes["pack"] == productmatch.OutcomeConflict {
			packOutcome = matchflow.PackOutcomeConflict
		} else {
			packOutcome = matchflow.PackOutcomeUnstated
		}
	}

	prodID := targetID
	return matchflow.RowDecision{
		V:         1,
		State:     matchflow.StateConfirmed,
		Basis:     matchflow.BasisReviewer,
		ProductID: &prodID,
		Granularity: matchflow.GranularityPair{
			Required: pol.Required.MatchflowGranularity(),
			Achieved: achieved,
		},
		Pack: matchflow.PackDecision{
			Outcome: packOutcome,
		},
		Reasons:  []matchflow.Reason{{Code: reasonCode}},
		Revision: a.ExpectedRev + 1,
		Versions: matchflow.Versions{
			Policy:          pol.Version,
			Normalizer:      productmatch.NormalizerVersion,
			CatalogRevision: catRev,
		},
		Actor: &matchflow.DecisionActor{
			UserID:      actorID,
			At:          now,
			Corrections: a.Corrections,
		},
		AI: matchflow.AIDecision{State: matchflow.AIStateOff},
	}, nil
}

// PreviewBulk calculates the total and affected row counts for a bulk review action (FR-046).
func (s *serviceImpl) PreviewBulk(ctx context.Context, sc Scope, a ReviewAction, rows []RowRef) (BulkPreview, error) {
	if sc.Workflow == "" {
		return BulkPreview{}, errors.New("matching: scope workflow is required")
	}

	targetSig := a.RowRef.QuestionSignature
	if targetSig == "" && a.Row.Name != "" {
		rowCorrections := toProductMatchCorrections(a.Corrections)
		targetFacts := productmatch.ExtractRowFacts(&a.Row, rowCorrections)
		targetSig = productmatch.QuestionSignature(targetFacts, a.Row.Name, productmatch.NormalizerVersion)
	}

	var preview BulkPreview
	for _, r := range rows {
		preview.Total++
		if r.ExplicitlyRejected {
			continue
		}
		if r.State == string(matchflow.StateNoCandidate) && r.ExplicitlyRejected {
			continue
		}
		if targetSig != "" && r.QuestionSignature == targetSig {
			preview.Affected++
		}
	}

	slog.InfoContext(ctx, "matching: bulk preview",
		"workflow", sc.Workflow,
		"effective_org", sc.EffectiveOrg,
		"action", a.Action,
		"target_sig", targetSig,
		"total", preview.Total,
		"affected", preview.Affected,
	)

	return preview, nil
}

func toProductMatchCorrections(in []matchflow.Correction) []productmatch.Correction {
	if len(in) == 0 {
		return nil
	}
	out := make([]productmatch.Correction, len(in))
	for i, c := range in {
		out[i] = productmatch.Correction{
			Attribute: c.Attribute,
			Value:     c.Value,
			Reason:    c.Reason,
		}
	}
	return out
}
