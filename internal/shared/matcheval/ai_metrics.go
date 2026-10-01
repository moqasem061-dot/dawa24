package matcheval

// ComputeAIMetrics calculates the net resolution gain, AI-added precision, token/cost metrics,
// and zero gain reasons according to contracts/match-eval-cli.md and research R16.
func ComputeAIMetrics(
	cases []Case,
	detOutcomes map[string]Outcome,
	aiOutcomes map[string]Outcome,
	totalAttempts, totalTokens int,
	totalCost float64,
	costKnown bool,
) AIStats {
	stats := AIStats{
		Attempts:        totalAttempts,
		Tokens:          totalTokens,
		Cost:            totalCost,
		CostKnown:       costKnown,
		ZeroGainReasons: make(map[string]int),
	}

	resolvableCount := 0
	for _, c := range cases {
		if c.Label.Disputed {
			continue
		}
		if c.Label.Resolvable {
			resolvableCount++
		}

		det, hasDet := detOutcomes[c.ID]
		ai, hasAI := aiOutcomes[c.ID]

		detAccepted := hasDet && isAcceptedState(det.State) && det.MatchedProductID != nil
		detCorrect := detAccepted && matchesTargets(*det.MatchedProductID, c.Label.Targets)

		aiAccepted := hasAI && isAcceptedState(ai.State) && ai.MatchedProductID != nil
		aiCorrect := aiAccepted && matchesTargets(*ai.MatchedProductID, c.Label.Targets)

		switch {
		case !detAccepted && aiCorrect:
			stats.GainNewCorrect++
		case !detAccepted && aiAccepted && !aiCorrect:
			stats.NewWrong++
		case detCorrect && (!aiAccepted || !aiCorrect):
			stats.LostCorrect++
		default:
			// No gain categories
			if detCorrect && aiCorrect {
				stats.ZeroGainReasons["already_settled_confirmed"]++
			} else if !aiAccepted {
				if hasAI && ai.State != "" {
					stats.ZeroGainReasons["abstained_"+ai.State]++
				} else {
					stats.ZeroGainReasons["unattempted_or_residue"]++
				}
			}
		}
	}

	netGain := stats.GainNewCorrect - stats.NewWrong - stats.LostCorrect
	if resolvableCount > 0 {
		stats.NetGainRate = float64(netGain) / float64(resolvableCount)
	}

	aiNewlyAccepted := stats.GainNewCorrect + stats.NewWrong
	if aiNewlyAccepted > 0 {
		stats.AIAddedPrecisionLB95 = ClopperPearsonLower(stats.GainNewCorrect, aiNewlyAccepted, 0.95)
	}

	if stats.GainNewCorrect > 0 {
		stats.CostPerVerifiedGain = stats.Cost / float64(stats.GainNewCorrect)
	}

	return stats
}

func isAcceptedState(st string) bool {
	return st == "accepted" || st == "confirmed"
}

func matchesTargets(prodID int64, targets []int64) bool {
	for _, t := range targets {
		if prodID == t {
			return true
		}
	}
	return false
}
