package ingest

import "github.com/moqasem061-dot/dawa24/internal/shared/matchflow"

func decisionMatchScore(dec matchflow.RowDecision) float64 {
	if id := matchflow.Columns(dec).MatchedProductID; id != nil {
		for _, candidate := range dec.Candidates {
			if candidate.ProductID == *id {
				return candidate.Score
			}
		}
		return 1 // Identifier/knowledge matches may have no scored candidates.
	}
	if len(dec.Candidates) > 0 {
		return dec.Candidates[0].Score
	}
	return 0
}

// RestoreDecisionScores repairs legacy score projections without changing reviewed links.
func (r *RowOutcome) RestoreDecisionScores() {
	if r.MatchDecision == nil || r.IsManuallyMatched {
		return
	}
	for i := range r.Candidates {
		for _, candidate := range r.MatchDecision.Candidates {
			if candidate.ProductID == r.Candidates[i].ProductID {
				r.Candidates[i].Score = candidate.Score
				if r.ProductID != nil && *r.ProductID == candidate.ProductID {
					r.MatchScore = candidate.Score
				}
				break
			}
		}
	}
}
