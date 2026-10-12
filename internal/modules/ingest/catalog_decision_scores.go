package ingest

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
