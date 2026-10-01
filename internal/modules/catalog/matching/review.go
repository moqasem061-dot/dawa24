package matching

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// Search queries the master catalog for candidate products matching q, presenting diffs and hard conflicts against row.
func (s *serviceImpl) Search(ctx context.Context, sc Scope, q string, row productmatch.Row, limit int) ([]matchflow.CandidateView, error) {
	if sc.Workflow == "" {
		return nil, errors.New("matching: scope workflow is required")
	}

	catRev, err := s.repo.CurrentRevision(ctx, "master")
	if err != nil {
		catRev = 1
	}

	idx, err := s.indexCache.Get(ctx, "master", catRev)
	if err != nil || idx == nil {
		return nil, errors.New("matching: catalog unavailable")
	}

	if limit <= 0 {
		limit = 10
	} else if limit > 50 {
		limit = 50
	}

	var candidateIDs []int64
	scores := make(map[int64]float64)
	seen := make(map[int64]bool)

	trimmedQ := strings.TrimSpace(q)
	if trimmedQ != "" {
		if id, parseErr := strconv.ParseInt(trimmedQ, 10, 64); parseErr == nil && id > 0 {
			if _, ok := idx.Lookup(id); ok {
				candidateIDs = append(candidateIDs, id)
				scores[id] = 1.0
				seen[id] = true
			}
		}

		searchRow := row
		searchRow.Name = trimmedQ
		searchRow.Barcode = trimmedQ
		searchRow.SKU = trimmedQ
		collectCandidates(idx, &searchRow, limit, &candidateIDs, scores, seen)
	} else if strings.TrimSpace(row.Name) != "" || row.Barcode != "" || row.SKU != "" {
		collectCandidates(idx, &row, limit, &candidateIDs, scores, seen)
	}

	rowFacts := productmatch.ExtractRowFacts(&row, nil)
	views := make([]matchflow.CandidateView, 0, len(candidateIDs))

	for _, id := range candidateIDs {
		if len(views) >= limit {
			break
		}
		p, ok := idx.Lookup(id)
		if !ok {
			continue
		}

		pFacts := p.Facts()
		comp := productmatch.CompareFacts(rowFacts, pFacts)
		eligible := len(comp.Hard) == 0

		hardConflicts := make([]string, len(comp.Hard))
		for i, h := range comp.Hard {
			hardConflicts[i] = string(h) + "_conflict"
		}

		diffMap := make(map[string]string, len(comp.Attributes))
		for k, v := range comp.Attributes {
			diffMap[k] = string(v)
		}

		views = append(views, matchflow.CandidateView{
			ProductID:     p.ID,
			NameAR:        p.NameAR,
			NameEN:        p.NameEN,
			DosageForm:    p.DosageForm,
			Concentration: p.Concentration,
			Unit:          p.Unit,
			Manufacturer:  p.Manufacturer,
			PublicPrice:   p.PublicPrice,
			Score:         scores[id],
			Eligible:      eligible,
			HardConflicts: hardConflicts,
			Diff:          diffMap,
		})
	}

	slog.InfoContext(ctx, "matching: search completed",
		"workflow", sc.Workflow,
		"effective_org", sc.EffectiveOrg,
		"query", q,
		"results", len(views),
	)

	return views, nil
}

func collectCandidates(
	idx *productmatch.Index,
	r *productmatch.Row,
	limit int,
	candidateIDs *[]int64,
	scores map[int64]float64,
	seen map[int64]bool,
) {
	opts := productmatch.MatchOptions{
		MinStrong:           0.1,
		MinReview:           0.05,
		PoolLimit:           400,
		MaxCandidates:       limit,
		TrustBarcode:        true,
		TrustSupplierCode:   true,
		CodeIsAuthoritative: true,
	}

	res := idx.Match(r, opts)
	if res.ProductID > 0 && !seen[res.ProductID] {
		*candidateIDs = append(*candidateIDs, res.ProductID)
		scores[res.ProductID] = res.Score
		seen[res.ProductID] = true
	}
	for _, c := range res.Candidates {
		if !seen[c.ProductID] {
			*candidateIDs = append(*candidateIDs, c.ProductID)
			scores[c.ProductID] = c.Score
			seen[c.ProductID] = true
		}
	}
}

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
