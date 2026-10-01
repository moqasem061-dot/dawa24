package matcheval

import "fmt"

// PromotePrompt evaluates whether a candidate prompt can be promoted over an incumbent
// prompt according to research R16 and FR-080:
// 1. Candidate must beat incumbent on net AI resolution gain (gain_new_correct - new_wrong - lost_correct).
// 2. Candidate must not lower AI-added precision's 95% lower bound.
// 3. Candidate must not raise mean output tokens by >20%.
// Length and stylistic sophistication are not criteria.
func PromotePrompt(incumbent, candidate Report) (bool, string) {
	if incumbent.Overall.AI == nil {
		return false, "incumbent report missing AI evaluation metrics"
	}
	if candidate.Overall.AI == nil {
		return false, "candidate report missing AI evaluation metrics"
	}

	incAI := incumbent.Overall.AI
	candAI := candidate.Overall.AI

	// 1. Net AI resolution gain check
	incNetGain := incAI.GainNewCorrect - incAI.NewWrong - incAI.LostCorrect
	candNetGain := candAI.GainNewCorrect - candAI.NewWrong - candAI.LostCorrect

	if candNetGain <= incNetGain {
		return false, fmt.Sprintf(
			"candidate net AI resolution gain (%d) does not beat incumbent (%d)",
			candNetGain, incNetGain,
		)
	}

	// 2. AI-added precision 95% lower bound check
	if candAI.AIAddedPrecisionLB95 < incAI.AIAddedPrecisionLB95 {
		return false, fmt.Sprintf(
			"candidate AI-added precision lower bound (%.4f) is lower than incumbent (%.4f)",
			candAI.AIAddedPrecisionLB95, incAI.AIAddedPrecisionLB95,
		)
	}

	// 3. Mean output tokens check (must not raise by >20%)
	if incAI.Attempts > 0 && candAI.Attempts > 0 && incAI.Tokens > 0 {
		incMeanTokens := float64(incAI.Tokens) / float64(incAI.Attempts)
		candMeanTokens := float64(candAI.Tokens) / float64(candAI.Attempts)
		if incMeanTokens > 0 {
			increase := (candMeanTokens - incMeanTokens) / incMeanTokens
			if increase > 0.20 {
				return false, fmt.Sprintf(
					"candidate mean tokens per attempt (%.1f) exceeds incumbent (%.1f) by %.1f%% (> 20%% limit)",
					candMeanTokens, incMeanTokens, increase*100,
				)
			}
		}
	}

	return true, fmt.Sprintf(
		"candidate qualified: net gain %d > %d, precision LB %.4f >= %.4f, token increase within budget",
		candNetGain, incNetGain, candAI.AIAddedPrecisionLB95, incAI.AIAddedPrecisionLB95,
	)
}
