package productmatch

import (
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/sheet"
)

// Decide evaluates a row against the catalog index under the given policy,
// ensuring all paths pass through Gate.Evaluate before acceptance.
func (idx *Index) Decide(row *Row, pol Policy) matchflow.RowDecision {
	opts := MatchOptions{
		MinStrong:     pol.MinStrong,
		MinReview:     pol.MinReview,
		PoolLimit:     400,
		MaxCandidates: 5,
	}
	dec, _ := idx.decideRow(row, pol, opts)
	return dec
}

func (idx *Index) decideRow(row *Row, pol Policy, opts MatchOptions) (matchflow.RowDecision, []scoredProduct) {
	catalogRev, normalizerVer := idx.Revision()
	versions := matchflow.Versions{
		Policy:          pol.Version,
		Normalizer:      normalizerVer,
		CatalogRevision: catalogRev,
	}

	if idx == nil || idx.total == 0 {
		return matchflow.RowDecision{
			V:        1,
			State:    matchflow.StateUnavailable,
			Basis:    matchflow.BasisSimilarity,
			Reasons:  []matchflow.Reason{{Code: string(ReasonCatalogUnavailable)}},
			Versions: versions,
			Revision: 1,
			AI:       matchflow.AIDecision{State: matchflow.AIStateOff},
		}, nil
	}

	rowFacts := ExtractRowFacts(row, nil)
	gate := NewGate(idx)

	// 1. Try Barcode / GTIN path if available and allowed
	if pol.Identifiers.AllowGTIN && row.Barcode != "" {
		code := sheet.DigitsOnly(row.Barcode)
		if hits := idx.byBarcode[code]; len(hits) > 0 {
			unique := len(hits) == 1
			valid := ValidGTIN(row.Barcode)
			if !valid && len(code) >= 8 {
				valid = true
			}
			prop := Proposal{
				Path:             PathIdentifier,
				ProductID:        hits[0].ID,
				Namespace:        string(NamespaceGTIN),
				IdentifierValid:  valid,
				IdentifierUnique: unique,
			}
			verdict := gate.Evaluate(rowFacts, prop, pol)
			if verdict.Eligible && verdict.State == matchflow.StateAccepted {
				return makeRowDecision(hits[0].ID, matchflow.BasisIdentifier, verdict, versions, pol, nil, nil), nil
			}
			// If not eligible (e.g. hard conflict like strength/form/brand), reject identifier proposal
			if len(verdict.Hard) > 0 {
				return makeRowDecision(hits[0].ID, matchflow.BasisIdentifier, verdict, versions, pol, nil, nil), nil
			}
		}
	}

	// 2. Try Catalog Code / Supplier Code if available and allowed
	if (pol.Identifiers.AllowCatalogCode || pol.Identifiers.AllowSupplierCode) && row.SKU != "" {
		key := sheet.NormalizeCode(row.SKU)
		if hits := idx.bySKU[key]; len(hits) > 0 {
			unique := len(hits) == 1
			ns := NamespaceCatalogCode
			if pol.Identifiers.AllowSupplierCode {
				ns = NamespaceSupplierCode
			}
			prop := Proposal{
				Path:             PathIdentifier,
				ProductID:        hits[0].ID,
				Namespace:        string(ns),
				IdentifierValid:  len(key) >= 4,
				IdentifierUnique: unique,
			}
			verdict := gate.Evaluate(rowFacts, prop, pol)
			if verdict.Eligible && verdict.State == matchflow.StateAccepted {
				return makeRowDecision(hits[0].ID, matchflow.BasisIdentifier, verdict, versions, pol, nil, nil), nil
			}
			if len(verdict.Hard) > 0 {
				return makeRowDecision(hits[0].ID, matchflow.BasisIdentifier, verdict, versions, pol, nil, nil), nil
			}
		}
	}

	// 3. Retrieval & Ranking
	q := idx.newQuery(row)
	if opts.PoolLimit <= 0 {
		opts.PoolLimit = 400
	}
	if opts.MaxCandidates <= 0 {
		opts.MaxCandidates = 5
	}
	scored := idx.score(q, opts)
	if len(scored) == 0 {
		return matchflow.RowDecision{
			V:        1,
			State:    matchflow.StateNoCandidate,
			Basis:    matchflow.BasisSimilarity,
			Reasons:  []matchflow.Reason{{Code: string(ReasonNoCandidate)}},
			Versions: versions,
			Revision: 1,
			AI:       matchflow.AIDecision{State: matchflow.AIStateOff},
		}, nil
	}

	// 4. Candidates & Siblings
	best := scored[0]
	candDecisions := make([]matchflow.CandidateDecision, 0, min(len(scored), opts.MaxCandidates))
	var siblingIDs []int64

	for i, s := range scored {
		sFacts := s.product.Facts()
		sComp := CompareFacts(rowFacts, sFacts)
		eligible := len(sComp.Hard) == 0
		if eligible {
			siblingIDs = append(siblingIDs, s.product.ID)
		}
		if i < opts.MaxCandidates {
			diffMap := make(map[string]string, len(sComp.Attributes))
			for k, v := range sComp.Attributes {
				diffMap[k] = string(v)
			}
			candDecisions = append(candDecisions, matchflow.CandidateDecision{
				ProductID: s.product.ID,
				Rank:      i + 1,
				Eligible:  eligible,
				Diff:      diffMap,
				Reasons:   conflictKindsToStrings(sComp.Hard),
			})
		}
	}

	bestPath := PathSimilarity
	bestBasis := matchflow.BasisSimilarity
	if best.exact {
		bestPath = PathExact
		bestBasis = matchflow.BasisExact
	}

	prop := Proposal{
		Path:        bestPath,
		ProductID:   best.product.ID,
		Score:       best.score,
		Siblings:    siblingIDs,
		LettersOnly: best.blind,
	}
	verdict := gate.Evaluate(rowFacts, prop, pol)

	var flags []string
	if q.poolTruncated && verdict.State != matchflow.StateAccepted {
		flags = append(flags, "retrieval_incomplete")
	}

	return makeRowDecision(best.product.ID, bestBasis, verdict, versions, pol, candDecisions, flags), scored
}

func makeRowDecision(
	productID int64,
	basis matchflow.Basis,
	verdict Verdict,
	versions matchflow.Versions,
	pol Policy,
	candidates []matchflow.CandidateDecision,
	flags []string,
) matchflow.RowDecision {
	var prodPtr *int64
	if productID > 0 {
		id := productID
		prodPtr = &id
	}

	reasons := make([]matchflow.Reason, 0, len(verdict.Reasons))
	for _, r := range verdict.Reasons {
		reasons = append(reasons, matchflow.Reason{
			Code:   string(r.Code),
			Params: r.Params,
		})
	}

	return matchflow.RowDecision{
		V:         1,
		State:     verdict.State,
		Basis:     basis,
		ProductID: prodPtr,
		Granularity: matchflow.GranularityPair{
			Required: pol.Required.MatchflowGranularity(),
			Achieved: verdict.Achieved.MatchflowGranularity(),
		},
		Pack: matchflow.PackDecision{
			Outcome: matchflow.PackOutcome(verdict.Pack),
		},
		Candidates: candidates,
		Reasons:    reasons,
		Flags:      flags,
		Versions:   versions,
		Revision:   1,
		AI:         matchflow.AIDecision{State: matchflow.AIStateOff},
	}
}

func conflictKindsToStrings(conflicts []ConflictKind) []string {
	if len(conflicts) == 0 {
		return nil
	}
	out := make([]string, len(conflicts))
	for i, c := range conflicts {
		out[i] = string(c) + "_conflict"
	}
	return out
}
